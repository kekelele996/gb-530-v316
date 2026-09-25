package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/model"
	"radiation-dose-budget-control/backend/internal/repository"
)

var adjustmentTestDBSeq int64

func newAdjustmentService(t *testing.T) (*TemporaryLimitAdjustmentService, *gorm.DB, model.WorkerProfile, dto.Actor, dto.Actor) {
	t.Helper()
	dsn := fmt.Sprintf("file:limit-adjustments-%d?mode=memory&cache=shared", atomic.AddInt64(&adjustmentTestDBSeq, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.WorkerProfile{}, &model.TemporaryLimitAdjustment{}, &model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	worker := model.WorkerProfile{
		WorkerCode: "RA-TEST", DisplayName: "Test Worker", AuthorizationLevel: "L2",
		AnnualLimitMSV: 20, AdministrativeLimitMSV: 12, ProfileStatus: constants.ProfileStatusActive,
		PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Version: 1,
	}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatalf("create worker: %v", err)
	}
	planner := dto.Actor{ID: 1, Username: "planner", Role: constants.RolePlanner}
	rpo := dto.Actor{ID: 2, Username: "rpo", Role: constants.RoleRPOReviewer}
	service := NewTemporaryLimitAdjustmentService(
		db,
		repository.NewTemporaryLimitAdjustmentRepository(db),
		repository.NewWorkerProfileRepository(db),
		NewAuditService(repository.NewSystemRepository(db)),
	)
	return service, db, worker, planner, rpo
}

func TestCreateAdjustmentRejectsOverLegalAndOverlap(t *testing.T) {
	service, _, worker, planner, _ := newAdjustmentService(t)
	day := func(value string) time.Time { parsed, _ := time.Parse("2006-01-02", value); return parsed.UTC() }

	// Adjusted limit above the legal limit is rejected.
	_, err := service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-10-01"), ExpiryDate: day("2026-10-10"),
		AdjustedLimitMSV: 20.5, Reason: "outage maintenance window",
	}, planner, "req-1")
	if err == nil {
		t.Fatal("expected adjustment above legal limit to be rejected")
	}
	if appErr, ok := err.(*AppError); !ok || appErr.Code != "adjustment_exceeds_legal_limit" {
		t.Fatalf("expected adjustment_exceeds_legal_limit, got %v", err)
	}

	created, err := service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-10-01"), ExpiryDate: day("2026-10-10"),
		AdjustedLimitMSV: 15, Reason: "outage maintenance window",
	}, planner, "req-2")
	if err != nil {
		t.Fatalf("create adjustment: %v", err)
	}
	if created.Status != constants.LimitAdjustmentStatusPending {
		t.Fatalf("status = %q, want pending", created.Status)
	}

	// Overlapping pending window cannot be submitted again.
	_, err = service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-10-05"), ExpiryDate: day("2026-10-12"),
		AdjustedLimitMSV: 14, Reason: "second overlapping window",
	}, planner, "req-3")
	if appErr, ok := err.(*AppError); !ok || appErr.Code != "limit_window_overlap" {
		t.Fatalf("expected limit_window_overlap, got %v", err)
	}

	// Adjacent windows that only touch at a boundary do not overlap:
	// [10-01,10-10) and [10-10,10-20) are disjoint.
	adjacent, err := service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-10-10"), ExpiryDate: day("2026-10-20"),
		AdjustedLimitMSV: 14, Reason: "follow-up window",
	}, planner, "req-4")
	if err != nil {
		t.Fatalf("adjacent window should be allowed: %v", err)
	}

	// After rejecting the first, a new overlapping proposal for that span is
	// allowed because rejected adjustments do not block resubmission.
	if _, err := service.Review(created.ID, dto.ReviewTemporaryLimitAdjustmentRequest{
		Decision: "reject", RejectionReason: "insufficient justification",
	}, dto.Actor{ID: 2, Username: "rpo", Role: constants.RoleRPOReviewer}, "req-5"); err != nil {
		t.Fatalf("reject adjustment: %v", err)
	}
	if _, err := service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-10-02"), ExpiryDate: day("2026-10-08"),
		AdjustedLimitMSV: 16, Reason: "resubmitted window",
	}, planner, "req-6"); err != nil {
		t.Fatalf("window overlapping only a rejected adjustment should be allowed: %v", err)
	}
	_ = adjacent
}

func TestReviewRequiresReasonForRejection(t *testing.T) {
	service, _, worker, planner, rpo := newAdjustmentService(t)
	day := func(value string) time.Time { parsed, _ := time.Parse("2006-01-02", value); return parsed.UTC() }

	created, err := service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-11-01"), ExpiryDate: day("2026-11-10"),
		AdjustedLimitMSV: 15, Reason: "maintenance",
	}, planner, "req-1")
	if err != nil {
		t.Fatalf("create adjustment: %v", err)
	}

	_, err = service.Review(created.ID, dto.ReviewTemporaryLimitAdjustmentRequest{Decision: "reject"}, rpo, "req-2")
	if appErr, ok := err.(*AppError); !ok || appErr.Code != "rejection_reason_required" {
		t.Fatalf("expected rejection_reason_required, got %v", err)
	}

	approved, err := service.Review(created.ID, dto.ReviewTemporaryLimitAdjustmentRequest{Decision: "approve"}, rpo, "req-3")
	if err != nil {
		t.Fatalf("approve adjustment: %v", err)
	}
	if approved.Status != constants.LimitAdjustmentStatusApproved || approved.ReviewedBy == nil {
		t.Fatalf("unexpected approved response: %+v", approved)
	}

	// An already-reviewed adjustment cannot be reviewed again.
	_, err = service.Review(created.ID, dto.ReviewTemporaryLimitAdjustmentRequest{
		Decision: "reject", RejectionReason: "too late",
	}, rpo, "req-4")
	if appErr, ok := err.(*AppError); !ok || appErr.Code != "invalid_state" {
		t.Fatalf("expected invalid_state on second review, got %v", err)
	}
}

func TestApprovedWindowBlocksOverlapAndAppliesByPeriodEnd(t *testing.T) {
	service, db, worker, planner, rpo := newAdjustmentService(t)
	repo := repository.NewTemporaryLimitAdjustmentRepository(db)
	day := func(value string) time.Time { parsed, _ := time.Parse("2006-01-02", value); return parsed.UTC() }

	created, err := service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-12-01"), ExpiryDate: day("2026-12-10"),
		AdjustedLimitMSV: 16, Reason: "overhaul",
	}, planner, "req-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := service.Review(created.ID, dto.ReviewTemporaryLimitAdjustmentRequest{Decision: "approve"}, rpo, "req-2"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// An approved window blocks a later overlapping proposal.
	_, err = service.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: day("2026-12-09"), ExpiryDate: day("2026-12-20"),
		AdjustedLimitMSV: 14, Reason: "overlap with approved",
	}, planner, "req-3")
	if appErr, ok := err.(*AppError); !ok || appErr.Code != "limit_window_overlap" {
		t.Fatalf("expected overlap with approved window, got %v", err)
	}

	// EffectiveAt uses the half-open [effective, expiry) window against the
	// supplied instant.
	if _, err := repo.EffectiveAt(worker.ID, day("2026-11-30")); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("adjustment must not apply before effective date, err=%v", err)
	}
	active, err := repo.EffectiveAt(worker.ID, day("2026-12-01"))
	if err != nil || active.AdjustedLimitMSV != 16 {
		t.Fatalf("adjustment must cover the effective date: %v", err)
	}
	if _, err := repo.EffectiveAt(worker.ID, day("2026-12-10")); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expiry date is excluded by the half-open window, err=%v", err)
	}
}
