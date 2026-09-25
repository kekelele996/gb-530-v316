package service

import (
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

var assessmentAdjustmentDBSeq int64

func ptrUint(value uint) *uint { return &value }

func newAssessmentAdjustmentFixture(t *testing.T) (*DoseBudgetAssessmentService, *TemporaryLimitAdjustmentService, *gorm.DB, model.WorkerProfile, model.WorkPermitPlan) {
	t.Helper()
	dsn := fmt.Sprintf("file:assessment-adjustment-%d?mode=memory&cache=shared", atomic.AddInt64(&assessmentAdjustmentDBSeq, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.WorkerProfile{}, &model.WorkPermitPlan{}, &model.ExposureEntry{},
		&model.DoseBudgetAssessment{}, &model.TemporaryLimitAdjustment{}, &model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	periodStart := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(24 * time.Hour)
	worker := model.WorkerProfile{
		WorkerCode: "RA-ASSESS", DisplayName: "Assess Worker", AuthorizationLevel: "L2",
		AnnualLimitMSV: 20, AdministrativeLimitMSV: 12, ProfileStatus: constants.ProfileStatusActive,
		PeriodStart: periodStart, Version: 1,
	}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatalf("create worker: %v", err)
	}
	plan := model.WorkPermitPlan{
		PlanCode: "ASSESS-1", WorkerID: worker.ID, WorkArea: "Annex", TaskCategory: "Survey",
		EstimatedRateMSVH: 0.3, PlannedMinutes: 60, ControlsJSON: `["briefing"]`,
		PermitStatus: constants.PermitStatusDraft, Version: 1, CreatedBy: 1,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	// Verified period dose of 12.7 mSv plus 0.3 mSv plan increment = 13 mSv projected.
	verifiedAt := time.Now().UTC().Add(-2 * time.Hour)
	entry := model.ExposureEntry{
		WorkerID: worker.ID, SourceRef: "TLD-ASSESS-1", OccurredAt: time.Now().UTC().Add(-5 * 24 * time.Hour),
		DoseMSV: 12.7, EntryType: constants.EntryTypeConfirmed, QualityFlag: constants.QualityFlagVerified,
		VerifiedBy: ptrUint(2), VerifiedAt: &verifiedAt, Note: "verified badge", CreatedBy: 1,
	}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatalf("create exposure entry: %v", err)
	}
	workerRepo := repository.NewWorkerProfileRepository(db)
	planRepo := repository.NewWorkPermitPlanRepository(db)
	entryRepo := repository.NewExposureEntryRepository(db)
	assessmentRepo := repository.NewDoseBudgetAssessmentRepository(db)
	adjustmentRepo := repository.NewTemporaryLimitAdjustmentRepository(db)
	systemRepo := repository.NewSystemRepository(db)
	audit := NewAuditService(systemRepo)
	assessmentService := NewDoseBudgetAssessmentService(
		db, assessmentRepo, planRepo, workerRepo, entryRepo, adjustmentRepo, audit, 0.9, "ALARA-2026.1",
	)
	adjustmentService := NewTemporaryLimitAdjustmentService(db, adjustmentRepo, workerRepo, audit)
	return assessmentService, adjustmentService, db, worker, plan
}

func TestAssessmentUsesApprovedAdjustmentOnlyWhenCoveringPeriodEnd(t *testing.T) {
	assessmentService, adjustmentService, _, worker, plan := newAssessmentAdjustmentFixture(t)
	planner := dto.Actor{ID: 1, Username: "planner", Role: constants.RolePlanner}
	rpo := dto.Actor{ID: 2, Username: "rpo", Role: constants.RoleRPOReviewer}
	periodEnd := time.Now().UTC().Add(-time.Hour).Truncate(time.Hour)

	// Baseline assessment: projected dose 13 mSv is above the 12 mSv admin limit.
	baseline, err := assessmentService.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: plan.ID, PeriodEnd: periodEnd, Version: 1,
	}, planner, "req-baseline")
	if err != nil {
		t.Fatalf("baseline assessment: %v", err)
	}
	if baseline.RiskBand != constants.DoseBandAboveAdmin {
		t.Fatalf("baseline risk band = %q, want above_admin", baseline.RiskBand)
	}
	if baseline.Evidence.LimitSource != "baseline_administrative" {
		t.Fatalf("baseline limit source = %q", baseline.Evidence.LimitSource)
	}

	// A pending adjustment must not affect assessments yet.
	effective := periodEnd.Add(-2 * 24 * time.Hour).Truncate(24 * time.Hour)
	expiry := periodEnd.Add(2 * 24 * time.Hour).Truncate(24 * time.Hour)
	pending, err := adjustmentService.Create(dto.CreateTemporaryLimitAdjustmentRequest{
		WorkerID: worker.ID, EffectiveDate: effective, ExpiryDate: expiry,
		AdjustedLimitMSV: 15, Reason: "overhaul window",
	}, planner, "req-request")
	if err != nil {
		t.Fatalf("request adjustment: %v", err)
	}
	pendingAssess, err := assessmentService.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: plan.ID, PeriodEnd: periodEnd, Version: 2,
	}, planner, "req-pending-assess")
	if err != nil {
		t.Fatalf("pending assessment: %v", err)
	}
	if pendingAssess.Evidence.LimitSource != "baseline_administrative" || pendingAssess.RiskBand != constants.DoseBandAboveAdmin {
		t.Fatalf("pending adjustment must not change the limit: source=%s band=%s", pendingAssess.Evidence.LimitSource, pendingAssess.RiskBand)
	}

	// Approve: reassessment at the same period end now uses 15 mSv, so 13 mSv
	// is within the temporary administrative limit.
	if _, err := adjustmentService.Review(pending.ID, dto.ReviewTemporaryLimitAdjustmentRequest{Decision: "approve"}, rpo, "req-approve"); err != nil {
		t.Fatalf("approve adjustment: %v", err)
	}
	approvedAssess, err := assessmentService.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: plan.ID, PeriodEnd: periodEnd, Version: 3,
	}, planner, "req-approved-assess")
	if err != nil {
		t.Fatalf("approved assessment: %v", err)
	}
	if approvedAssess.Evidence.LimitSource != "approved_temporary_adjustment" {
		t.Fatalf("limit source = %q, want approved_temporary_adjustment", approvedAssess.Evidence.LimitSource)
	}
	if approvedAssess.Evidence.AdministrativeLimit != 15 || approvedAssess.Evidence.BaselineAdminLimit != 12 {
		t.Fatalf("unexpected limits in evidence: %+v", approvedAssess.Evidence)
	}
	if approvedAssess.Evidence.LimitAdjustmentID != pending.ID {
		t.Fatalf("evidence adjustment id = %d, want %d", approvedAssess.Evidence.LimitAdjustmentID, pending.ID)
	}
	if approvedAssess.RiskBand != constants.DoseBandWithinAdmin {
		t.Fatalf("risk band = %q, want within_admin under the temporary limit", approvedAssess.RiskBand)
	}

	// A period end outside the approved window keeps using the baseline limit.
	outside, err := assessmentService.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: plan.ID, PeriodEnd: effective.Add(-24 * time.Hour), Version: 4,
	}, planner, "req-outside-assess")
	if err != nil {
		t.Fatalf("outside-window assessment: %v", err)
	}
	if outside.Evidence.LimitSource != "baseline_administrative" {
		t.Fatalf("limit source outside window = %q", outside.Evidence.LimitSource)
	}

	// Previously stored assessments are immutable: baseline evidence still
	// shows the original limit and band.
	stored, err := assessmentService.Get(baseline.ID)
	if err != nil {
		t.Fatalf("load stored baseline: %v", err)
	}
	if stored.Evidence.AdministrativeLimit != 12 || stored.RiskBand != constants.DoseBandAboveAdmin {
		t.Fatalf("historical assessment changed: limit=%v band=%s", stored.Evidence.AdministrativeLimit, stored.RiskBand)
	}
}
