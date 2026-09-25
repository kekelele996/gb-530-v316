package service

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/model"
	"radiation-dose-budget-control/backend/internal/repository"
)

type TemporaryLimitAdjustmentService struct {
	db          *gorm.DB
	adjustments *repository.TemporaryLimitAdjustmentRepository
	workers     *repository.WorkerProfileRepository
	audit       *AuditService
}

func NewTemporaryLimitAdjustmentService(
	db *gorm.DB,
	adjustments *repository.TemporaryLimitAdjustmentRepository,
	workers *repository.WorkerProfileRepository,
	audit *AuditService,
) *TemporaryLimitAdjustmentService {
	return &TemporaryLimitAdjustmentService{db: db, adjustments: adjustments, workers: workers, audit: audit}
}

func (service *TemporaryLimitAdjustmentService) Create(request dto.CreateTemporaryLimitAdjustmentRequest, actor dto.Actor, requestID string) (dto.TemporaryLimitAdjustmentResponse, error) {
	var response dto.TemporaryLimitAdjustmentResponse
	err := service.db.Transaction(func(tx *gorm.DB) error {
		adjustments := service.adjustments.WithDB(tx)
		workers := service.workers.WithDB(tx)
		worker, err := workers.FindForUpdate(request.WorkerID)
		if err != nil {
			return MapRepositoryError("worker profile", err)
		}
		effective := request.EffectiveDate.UTC().Truncate(24 * time.Hour)
		expiry := request.ExpiryDate.UTC().Truncate(24 * time.Hour)
		if !expiry.After(effective) {
			return BadRequest("invalid_adjustment_window", "expiry_date must be after effective_date")
		}
		if expiry.Sub(effective) > 370*24*time.Hour {
			return BadRequest("invalid_adjustment_window", "adjustment window cannot exceed 370 days")
		}
		if !expiry.After(time.Now().UTC()) {
			return BadRequest("invalid_adjustment_window", "expiry_date must remain in the future")
		}
		reason := strings.TrimSpace(request.Reason)
		adjusted := request.AdjustedLimitMSV
		if adjusted <= 0 || adjusted > worker.AnnualLimitMSV {
			return BadRequest("adjustment_exceeds_legal_limit", "adjusted_limit_msv must be positive and cannot exceed the worker's annual legal limit")
		}
		overlaps, err := adjustments.Overlapping(worker.ID, effective, expiry)
		if err != nil {
			return Internal("could not inspect adjustment windows", err)
		}
		if len(overlaps) > 0 {
			return Conflict("limit_window_overlap", "a pending or approved temporary adjustment already covers part of this window", nil)
		}
		adjustment := model.TemporaryLimitAdjustment{
			WorkerID: worker.ID, Status: constants.LimitAdjustmentStatusPending,
			EffectiveDate: effective, ExpiryDate: expiry, AdjustedLimitMSV: adjusted,
			Reason: reason, CreatedBy: actor.ID, CreatedAt: time.Now().UTC(),
		}
		if err := adjustments.Create(&adjustment); err != nil {
			return Internal("could not create temporary limit adjustment", err)
		}
		if err := service.audit.RecordTx(tx, actor, requestID, "limit_adjustment.requested", "temporary_limit_adjustment", auditID(adjustment.ID),
			map[string]any{
				"worker_id": worker.ID, "effective_date": effective, "expiry_date": expiry,
				"adjusted_limit_msv": adjusted, "baseline_admin_limit_msv": worker.AdministrativeLimitMSV,
				"legal_limit_msv": worker.AnnualLimitMSV, "reason_length": len(reason),
			}, nil, map[string]any{"status": adjustment.Status}); err != nil {
			return err
		}
		response = adjustmentResponse(adjustment, worker)
		return nil
	})
	if err != nil {
		return dto.TemporaryLimitAdjustmentResponse{}, err
	}
	return response, nil
}

func (service *TemporaryLimitAdjustmentService) Review(id uint, request dto.ReviewTemporaryLimitAdjustmentRequest, actor dto.Actor, requestID string) (dto.TemporaryLimitAdjustmentResponse, error) {
	var response dto.TemporaryLimitAdjustmentResponse
	err := service.db.Transaction(func(tx *gorm.DB) error {
		adjustments := service.adjustments.WithDB(tx)
		workers := service.workers.WithDB(tx)
		adjustment, err := adjustments.FindForUpdate(id)
		if err != nil {
			return MapRepositoryError("temporary limit adjustment", err)
		}
		if adjustment.Status != constants.LimitAdjustmentStatusPending {
			return Conflict("invalid_state", "only pending temporary limit adjustments can be reviewed", nil)
		}
		target := constants.LimitAdjustmentStatusApproved
		rejectionReason := ""
		if request.Decision == "reject" {
			target = constants.LimitAdjustmentStatusRejected
			rejectionReason = strings.TrimSpace(request.RejectionReason)
			if rejectionReason == "" {
				return BadRequest("rejection_reason_required", "rejection_reason is required when rejecting a temporary limit adjustment")
			}
		}
		worker, err := workers.FindForUpdate(adjustment.WorkerID)
		if err != nil {
			return MapRepositoryError("worker profile", err)
		}
		// The legal ceiling is re-checked at review time: a profile change between
		// request and decision must not let an approved override pass the legal limit.
		if request.Decision == "approve" && adjustment.AdjustedLimitMSV > worker.AnnualLimitMSV {
			return Conflict("adjustment_exceeds_legal_limit", "the worker's legal limit changed; the adjusted value no longer fits under it", nil)
		}
		now := time.Now().UTC()
		if err := adjustments.Review(id, constants.LimitAdjustmentStatusPending, target, actor.ID, now, rejectionReason); err != nil {
			return Conflict("state_conflict", "adjustment changed before review", err)
		}
		adjustment.Status = target
		adjustment.ReviewedBy = &actor.ID
		adjustment.ReviewedAt = &now
		adjustment.RejectionReason = rejectionReason
		if err := service.audit.RecordTx(tx, actor, requestID, "limit_adjustment.reviewed", "temporary_limit_adjustment", auditID(id),
			map[string]any{
				"decision": request.Decision, "worker_id": adjustment.WorkerID,
				"adjusted_limit_msv":      adjustment.AdjustedLimitMSV,
				"rejection_reason_length": len(rejectionReason),
			},
			map[string]any{"status": constants.LimitAdjustmentStatusPending}, map[string]any{"status": target}); err != nil {
			return err
		}
		response = adjustmentResponse(adjustment, worker)
		return nil
	})
	if err != nil {
		return dto.TemporaryLimitAdjustmentResponse{}, err
	}
	return response, nil
}

func (service *TemporaryLimitAdjustmentService) Get(id uint) (dto.TemporaryLimitAdjustmentResponse, error) {
	adjustment, err := service.adjustments.Find(id)
	if err != nil {
		return dto.TemporaryLimitAdjustmentResponse{}, MapRepositoryError("temporary limit adjustment", err)
	}
	worker, err := service.workers.Find(adjustment.WorkerID)
	if err != nil {
		return dto.TemporaryLimitAdjustmentResponse{}, MapRepositoryError("worker profile", err)
	}
	return adjustmentResponse(adjustment, worker), nil
}

func (service *TemporaryLimitAdjustmentService) List(page, pageSize int, workerFilter, status string) ([]dto.TemporaryLimitAdjustmentResponse, dto.PageMeta, error) {
	workerID, err := parseUintFilter(workerFilter)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	if status != "" && !constants.IsLimitAdjustmentStatus(status) {
		return nil, dto.PageMeta{}, BadRequest("invalid_adjustment_status", "status filter is not recognized")
	}
	adjustments, total, err := service.adjustments.List(page, pageSize, workerID, status)
	if err != nil {
		return nil, dto.PageMeta{}, Internal("could not list temporary limit adjustments", err)
	}
	responses := make([]dto.TemporaryLimitAdjustmentResponse, 0, len(adjustments))
	for _, adjustment := range adjustments {
		worker, err := service.workers.Find(adjustment.WorkerID)
		if err != nil {
			return nil, dto.PageMeta{}, MapRepositoryError("worker profile", err)
		}
		responses = append(responses, adjustmentResponse(adjustment, worker))
	}
	return responses, pageMeta(page, pageSize, total), nil
}

func adjustmentResponse(adjustment model.TemporaryLimitAdjustment, worker model.WorkerProfile) dto.TemporaryLimitAdjustmentResponse {
	return dto.TemporaryLimitAdjustmentResponse{
		ID: adjustment.ID, WorkerID: adjustment.WorkerID, WorkerCode: worker.WorkerCode, WorkerName: worker.DisplayName,
		Status: adjustment.Status, EffectiveDate: adjustment.EffectiveDate, ExpiryDate: adjustment.ExpiryDate,
		AdjustedLimitMSV: adjustment.AdjustedLimitMSV, Reason: adjustment.Reason, CreatedBy: adjustment.CreatedBy,
		CreatedAt: adjustment.CreatedAt, ReviewedBy: adjustment.ReviewedBy, ReviewedAt: adjustment.ReviewedAt,
		RejectionReason: adjustment.RejectionReason,
	}
}
