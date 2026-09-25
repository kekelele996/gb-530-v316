package service

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/model"
	"radiation-dose-budget-control/backend/internal/repository"
)

type adjustmentFixture struct {
	db          *gorm.DB
	adjustments *LimitAdjustmentService
	assessments *DoseBudgetAssessmentService
	worker      model.WorkerProfile
	planner     dto.Actor
	rpo         dto.Actor
}

func newAdjustmentFixture(t *testing.T) *adjustmentFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.WorkerProfile{}, &model.WorkPermitPlan{},
		&model.ExposureEntry{}, &model.DoseBudgetAssessment{}, &model.LimitAdjustment{}, &model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	workerRepository := repository.NewWorkerProfileRepository(db)
	planRepository := repository.NewWorkPermitPlanRepository(db)
	exposureRepository := repository.NewExposureEntryRepository(db)
	assessmentRepository := repository.NewDoseBudgetAssessmentRepository(db)
	adjustmentRepository := repository.NewLimitAdjustmentRepository(db)
	audit := NewAuditService(repository.NewSystemRepository(db))
	worker := model.WorkerProfile{
		WorkerCode: "QA-ADJ-530", DisplayName: "Adjustment QA", AuthorizationLevel: "Controlled area QA",
		AnnualLimitMSV: 10, AdministrativeLimitMSV: 5, ProfileStatus: constants.ProfileStatusActive,
		PeriodStart: time.Now().UTC().Add(-30 * 24 * time.Hour), Version: 1,
	}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatalf("create worker: %v", err)
	}
	return &adjustmentFixture{
		db:          db,
		adjustments: NewLimitAdjustmentService(db, adjustmentRepository, workerRepository, audit),
		assessments: NewDoseBudgetAssessmentService(db, assessmentRepository, planRepository, workerRepository, exposureRepository, adjustmentRepository, audit, 0.9, "ALARA-TEST"),
		worker:      worker,
		planner:     dto.Actor{ID: 1, Username: "planner", Role: constants.RolePlanner},
		rpo:         dto.Actor{ID: 2, Username: "rpo", Role: constants.RoleRPOReviewer},
	}
}

func (fixture *adjustmentFixture) createPlan(t *testing.T, code string) model.WorkPermitPlan {
	t.Helper()
	plan := model.WorkPermitPlan{
		PlanCode: code, WorkerID: fixture.worker.ID, WorkArea: "QA bay", TaskCategory: "Source check",
		EstimatedRateMSVH: 2.0, PlannedMinutes: 60, ControlsJSON: `["distance markers"]`,
		PermitStatus: constants.PermitStatusDraft, Version: 1, CreatedBy: fixture.planner.ID,
	}
	if err := fixture.db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return plan
}

func (fixture *adjustmentFixture) adjustmentRequest(from, to time.Time, limit float64) dto.CreateLimitAdjustmentRequest {
	return dto.CreateLimitAdjustmentRequest{
		EffectiveFrom: from, EffectiveTo: to, AdjustedLimitMSV: limit, Reason: "Outage maintenance window",
	}
}

func TestLimitAdjustmentReviewWorkflow(t *testing.T) {
	fixture := newAdjustmentFixture(t)
	now := time.Now().UTC()
	from, to := now.Add(-2*24*time.Hour), now.Add(2*24*time.Hour)

	created, err := fixture.adjustments.Create(fixture.worker.ID, fixture.adjustmentRequest(from, to, 8), fixture.planner, "req-create")
	if err != nil {
		t.Fatalf("create adjustment: %v", err)
	}
	if created.Status != constants.AdjustmentStatusPending {
		t.Fatalf("status = %q, want pending", created.Status)
	}

	if _, err := fixture.adjustments.Create(fixture.worker.ID, fixture.adjustmentRequest(now.Add(-24*time.Hour), now.Add(3*24*time.Hour), 7), fixture.planner, "req-overlap"); err == nil {
		t.Fatal("overlapping pending window was accepted")
	} else if appError, ok := err.(*AppError); !ok || appError.Code != "adjustment_overlap" {
		t.Fatalf("overlap error = %v, want adjustment_overlap", err)
	}

	if _, err := fixture.adjustments.Create(fixture.worker.ID, fixture.adjustmentRequest(now.Add(10*24*time.Hour), now.Add(12*24*time.Hour), 11), fixture.planner, "req-legal"); err == nil {
		t.Fatal("adjustment above the regulatory limit was accepted")
	} else if appError, ok := err.(*AppError); !ok || appError.Code != "limit_exceeds_legal" {
		t.Fatalf("legal cap error = %v, want limit_exceeds_legal", err)
	}

	if _, err := fixture.adjustments.Review(created.ID, dto.ReviewLimitAdjustmentRequest{Decision: "reject"}, fixture.rpo, "req-reject-empty"); err == nil {
		t.Fatal("rejection without a reason was accepted")
	} else if appError, ok := err.(*AppError); !ok || appError.Code != "review_note_required" {
		t.Fatalf("missing note error = %v, want review_note_required", err)
	}

	rejected, err := fixture.adjustments.Review(created.ID, dto.ReviewLimitAdjustmentRequest{Decision: "reject", Note: "Window not justified by outage plan"}, fixture.rpo, "req-reject")
	if err != nil {
		t.Fatalf("reject adjustment: %v", err)
	}
	if rejected.Status != constants.AdjustmentStatusRejected || rejected.ReviewNote == "" {
		t.Fatalf("rejected adjustment = %+v", rejected)
	}

	if _, err := fixture.adjustments.Review(created.ID, dto.ReviewLimitAdjustmentRequest{Decision: "approve"}, fixture.rpo, "req-reviewer-twice"); err == nil {
		t.Fatal("reviewed adjustment accepted a second review")
	}

	// A rejected adjustment does not block a new submission for the same window.
	resubmitted, err := fixture.adjustments.Create(fixture.worker.ID, fixture.adjustmentRequest(from, to, 8), fixture.planner, "req-resubmit")
	if err != nil {
		t.Fatalf("resubmit after rejection: %v", err)
	}
	approved, err := fixture.adjustments.Review(resubmitted.ID, dto.ReviewLimitAdjustmentRequest{Decision: "approve", Note: "Outage plan verified"}, fixture.rpo, "req-approve")
	if err != nil {
		t.Fatalf("approve adjustment: %v", err)
	}
	if approved.Status != constants.AdjustmentStatusApproved || approved.ReviewedBy == nil {
		t.Fatalf("approved adjustment = %+v", approved)
	}

	if _, err := fixture.adjustments.Create(fixture.worker.ID, fixture.adjustmentRequest(from, to, 9), fixture.planner, "req-overlap-approved"); err == nil {
		t.Fatal("window overlapping an approved adjustment was accepted")
	} else if appError, ok := err.(*AppError); !ok || appError.Code != "adjustment_overlap" {
		t.Fatalf("approved overlap error = %v, want adjustment_overlap", err)
	}

	listed, err := fixture.adjustments.List(fixture.worker.ID)
	if err != nil {
		t.Fatalf("list adjustments: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d adjustments, want 2", len(listed))
	}
}

func TestAssessmentAdoptsApprovedAdjustment(t *testing.T) {
	fixture := newAdjustmentFixture(t)
	now := time.Now().UTC()

	// Baseline assessment without any adjustment uses the profile limit.
	plan := fixture.createPlan(t, "QA-ADJ-PLAN-A")
	baseline, err := fixture.assessments.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: plan.ID, PeriodEnd: now, Version: plan.Version,
	}, fixture.planner, "req-baseline")
	if err != nil {
		t.Fatalf("baseline assessment: %v", err)
	}
	if baseline.Evidence.AdministrativeLimit != 5 || baseline.Evidence.LimitAdjustment != nil {
		t.Fatalf("baseline evidence = %+v", baseline.Evidence)
	}
	if baseline.RemainingAdminMSV != 3 || baseline.RiskBand != constants.DoseBandWithinAdmin {
		t.Fatalf("baseline margin/band = %v/%q", baseline.RemainingAdminMSV, baseline.RiskBand)
	}

	// Approve an adjustment covering the period end; new assessments adopt it.
	adjustment, err := fixture.adjustments.Create(fixture.worker.ID,
		fixture.adjustmentRequest(now.Add(-24*time.Hour), now.Add(24*time.Hour), 1.5), fixture.planner, "req-adjust")
	if err != nil {
		t.Fatalf("create adjustment: %v", err)
	}
	if _, err := fixture.adjustments.Review(adjustment.ID, dto.ReviewLimitAdjustmentRequest{Decision: "approve"}, fixture.rpo, "req-approve"); err != nil {
		t.Fatalf("approve adjustment: %v", err)
	}

	adjustedPlan := fixture.createPlan(t, "QA-ADJ-PLAN-B")
	adjusted, err := fixture.assessments.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: adjustedPlan.ID, PeriodEnd: now, Version: adjustedPlan.Version,
	}, fixture.planner, "req-adjusted")
	if err != nil {
		t.Fatalf("adjusted assessment: %v", err)
	}
	if adjusted.Evidence.AdministrativeLimit != 1.5 || adjusted.Evidence.BaseAdministrativeLimit != 5 {
		t.Fatalf("adjusted evidence limits = %+v", adjusted.Evidence)
	}
	if adjusted.Evidence.LimitAdjustment == nil || adjusted.Evidence.LimitAdjustment.ID != adjustment.ID {
		t.Fatalf("adjusted evidence adoption = %+v", adjusted.Evidence.LimitAdjustment)
	}
	if adjusted.RiskBand != constants.DoseBandAboveAdmin || adjusted.RemainingAdminMSV != 0 {
		t.Fatalf("adjusted margin/band = %v/%q", adjusted.RemainingAdminMSV, adjusted.RiskBand)
	}

	// A period end outside the validity window keeps the profile limit.
	outsidePlan := fixture.createPlan(t, "QA-ADJ-PLAN-C")
	outside, err := fixture.assessments.Assess(dto.CreateDoseBudgetAssessmentRequest{
		PlanID: outsidePlan.ID, PeriodEnd: now.Add(-3 * 24 * time.Hour), Version: outsidePlan.Version,
	}, fixture.planner, "req-outside")
	if err != nil {
		t.Fatalf("outside-window assessment: %v", err)
	}
	if outside.Evidence.AdministrativeLimit != 5 || outside.Evidence.LimitAdjustment != nil {
		t.Fatalf("outside-window evidence = %+v", outside.Evidence)
	}

	// Existing assessment results stay frozen with their original evidence.
	reloaded, err := fixture.assessments.Get(baseline.ID)
	if err != nil {
		t.Fatalf("reload baseline assessment: %v", err)
	}
	if reloaded.Evidence.AdministrativeLimit != 5 || reloaded.Evidence.LimitAdjustment != nil {
		t.Fatalf("frozen baseline evidence changed: %+v", reloaded.Evidence)
	}
}
