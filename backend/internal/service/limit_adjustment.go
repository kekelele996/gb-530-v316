package service

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/dosebudget"
	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/model"
	"radiation-dose-budget-control/backend/internal/repository"
)

type LimitAdjustmentService struct {
	db          *gorm.DB
	adjustments *repository.LimitAdjustmentRepository
	workers     *repository.WorkerProfileRepository
	audit       *AuditService
}

func NewLimitAdjustmentService(
	db *gorm.DB,
	adjustments *repository.LimitAdjustmentRepository,
	workers *repository.WorkerProfileRepository,
	audit *AuditService,
) *LimitAdjustmentService {
	return &LimitAdjustmentService{db: db, adjustments: adjustments, workers: workers, audit: audit}
}

func (service *LimitAdjustmentService) Create(
	workerID uint,
	request dto.CreateLimitAdjustmentRequest,
	actor dto.Actor,
	requestID string,
) (dto.LimitAdjustmentResponse, error) {
	window, err := dosebudget.NewAdjustmentWindow(request.EffectiveFrom, request.EffectiveTo)
	if err != nil {
		return dto.LimitAdjustmentResponse{}, BadRequest("invalid_adjustment_window", err.Error())
	}
	var response dto.LimitAdjustmentResponse
	err = service.db.Transaction(func(tx *gorm.DB) error {
		workers := service.workers.WithDB(tx)
		adjustments := service.adjustments.WithDB(tx)
		worker, err := workers.FindForUpdate(workerID)
		if err != nil {
			return MapRepositoryError("worker profile", err)
		}
		if request.AdjustedLimitMSV > worker.AnnualLimitMSV {
			return BadRequest("limit_exceeds_legal", "adjusted_limit_msv must not exceed the worker regulatory annual_limit_msv")
		}
		overlap, err := adjustments.HasOverlappingActive(worker.ID, window.From, window.To)
		if err != nil {
			return Internal("could not inspect overlapping adjustments", err)
		}
		if overlap {
			return Conflict("adjustment_overlap", "a pending or approved adjustment already overlaps this validity window", nil)
		}
		adjustment := model.LimitAdjustment{
			WorkerID: worker.ID, EffectiveFrom: window.From, EffectiveTo: window.To,
			AdjustedLimitMSV: request.AdjustedLimitMSV, Reason: strings.TrimSpace(request.Reason),
			Status: constants.AdjustmentStatusPending, CreatedBy: actor.ID,
		}
		if err := adjustments.Create(&adjustment); err != nil {
			return Internal("could not create limit adjustment", err)
		}
		if err := service.audit.RecordTx(tx, actor, requestID, "adjustment.submitted", "limit_adjustment", auditID(adjustment.ID),
			map[string]any{
				"worker_id": worker.ID, "effective_from": adjustment.EffectiveFrom, "effective_to": adjustment.EffectiveTo,
				"adjusted_limit_msv": adjustment.AdjustedLimitMSV, "base_administrative_limit_msv": worker.AdministrativeLimitMSV,
			}, nil, adjustmentAudit(adjustment)); err != nil {
			return err
		}
		response = adjustmentResponse(adjustment, worker)
		return nil
	})
	if err != nil {
		return dto.LimitAdjustmentResponse{}, err
	}
	return response, nil
}

func (service *LimitAdjustmentService) Review(
	id uint,
	request dto.ReviewLimitAdjustmentRequest,
	actor dto.Actor,
	requestID string,
) (dto.LimitAdjustmentResponse, error) {
	note := strings.TrimSpace(request.Note)
	if request.Decision == "reject" && note == "" {
		return dto.LimitAdjustmentResponse{}, BadRequest("review_note_required", "a rejection reason is required when rejecting an adjustment")
	}
	var response dto.LimitAdjustmentResponse
	err := service.db.Transaction(func(tx *gorm.DB) error {
		adjustments := service.adjustments.WithDB(tx)
		adjustment, err := adjustments.FindForUpdate(id)
		if err != nil {
			return MapRepositoryError("limit adjustment", err)
		}
		if adjustment.Status != constants.AdjustmentStatusPending {
			return Conflict("invalid_state", "only pending adjustments can be reviewed", nil)
		}
		target := constants.AdjustmentStatusRejected
		if request.Decision == "approve" {
			target = constants.AdjustmentStatusApproved
		}
		now := time.Now().UTC()
		if err := adjustments.Review(id, constants.AdjustmentStatusPending, target, actor.ID, now, note); err != nil {
			return Conflict("state_conflict", "adjustment changed before review", err)
		}
		before := adjustment
		adjustment.Status = target
		adjustment.ReviewedBy = &actor.ID
		adjustment.ReviewedAt = &now
		adjustment.ReviewNote = note
		if err := service.audit.RecordTx(tx, actor, requestID, "adjustment.reviewed", "limit_adjustment", auditID(id),
			map[string]any{"decision": request.Decision, "review_note_length": len(note), "planning_only": true},
			adjustmentAudit(before), adjustmentAudit(adjustment)); err != nil {
			return err
		}
		worker, err := service.workers.WithDB(tx).Find(adjustment.WorkerID)
		if err != nil {
			return MapRepositoryError("adjustment worker", err)
		}
		response = adjustmentResponse(adjustment, worker)
		return nil
	})
	if err != nil {
		return dto.LimitAdjustmentResponse{}, err
	}
	return response, nil
}

func (service *LimitAdjustmentService) List(workerID uint) ([]dto.LimitAdjustmentResponse, error) {
	worker, err := service.workers.Find(workerID)
	if err != nil {
		return nil, MapRepositoryError("worker profile", err)
	}
	adjustments, err := service.adjustments.ListForWorker(worker.ID)
	if err != nil {
		return nil, Internal("could not list limit adjustments", err)
	}
	responses := make([]dto.LimitAdjustmentResponse, 0, len(adjustments))
	for _, adjustment := range adjustments {
		responses = append(responses, adjustmentResponse(adjustment, worker))
	}
	return responses, nil
}

func adjustmentResponse(adjustment model.LimitAdjustment, worker model.WorkerProfile) dto.LimitAdjustmentResponse {
	return dto.LimitAdjustmentResponse{
		ID: adjustment.ID, WorkerID: adjustment.WorkerID, WorkerCode: worker.WorkerCode, WorkerName: worker.DisplayName,
		EffectiveFrom: adjustment.EffectiveFrom, EffectiveTo: adjustment.EffectiveTo,
		AdjustedLimitMSV: adjustment.AdjustedLimitMSV, Reason: adjustment.Reason, Status: adjustment.Status,
		ReviewedBy: adjustment.ReviewedBy, ReviewedAt: adjustment.ReviewedAt, ReviewNote: adjustment.ReviewNote,
		CreatedBy: adjustment.CreatedBy, CreatedAt: adjustment.CreatedAt,
	}
}

func adjustmentAudit(adjustment model.LimitAdjustment) map[string]any {
	return map[string]any{
		"id": adjustment.ID, "worker_id": adjustment.WorkerID, "effective_from": adjustment.EffectiveFrom,
		"effective_to": adjustment.EffectiveTo, "adjusted_limit_msv": adjustment.AdjustedLimitMSV,
		"status": adjustment.Status, "reviewed_by": adjustment.ReviewedBy, "reviewed_at": adjustment.ReviewedAt,
	}
}
