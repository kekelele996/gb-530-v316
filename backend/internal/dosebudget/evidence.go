package dosebudget

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const BoundaryStatement = "Offline ALARA planning result only. It is not a work permit, dosimeter reading, regulatory determination, or medical advice."

// AdjustmentRef freezes the approved temporary administrative limit adjustment
// that was adopted for an assessment, so the evidence stays replayable.
type AdjustmentRef struct {
	ID               uint      `json:"id"`
	EffectiveFrom    time.Time `json:"effective_from"`
	EffectiveTo      time.Time `json:"effective_to"`
	AdjustedLimitMSV float64   `json:"adjusted_limit_msv"`
}

type Snapshot struct {
	WorkerID                   uint           `json:"worker_id"`
	WorkerCode                 string         `json:"worker_code"`
	WorkerVersion              uint           `json:"worker_version"`
	PlanID                     uint           `json:"plan_id"`
	PlanCode                   string         `json:"plan_code"`
	PlanVersion                uint           `json:"plan_version"`
	PeriodStart                time.Time      `json:"period_start"`
	PeriodEnd                  time.Time      `json:"period_end"`
	EstimatedRateMSVH          float64        `json:"estimated_rate_msvh"`
	PlannedMinutes             int            `json:"planned_minutes"`
	Controls                   []string       `json:"controls"`
	IncludedExposureIDs        []uint         `json:"included_exposure_ids"`
	ExcludedExposureIDs        []uint         `json:"excluded_exposure_ids"`
	AdministrativeLimitMSV     float64        `json:"administrative_limit_msv"`
	BaseAdministrativeLimitMSV float64        `json:"base_administrative_limit_msv"`
	LimitAdjustment            *AdjustmentRef `json:"limit_adjustment,omitempty"`
	LegalLimitMSV              float64        `json:"legal_limit_msv"`
	NearLegalRatio             float64        `json:"near_legal_ratio"`
	ThresholdVersion           string         `json:"threshold_version"`
}

type Evidence struct {
	PeriodStart             time.Time      `json:"period_start"`
	PeriodEnd               time.Time      `json:"period_end"`
	VerifiedEntryCount      int            `json:"verified_entry_count"`
	ExcludedEntryCount      int            `json:"excluded_entry_count"`
	CorrectedChainCount     int            `json:"corrected_chain_count"`
	Formula                 string         `json:"formula"`
	ProjectionFormula       string         `json:"projection_formula"`
	AdministrativeLimit     float64        `json:"administrative_limit_msv"`
	BaseAdministrativeLimit float64        `json:"base_administrative_limit_msv"`
	LimitAdjustment         *AdjustmentRef `json:"limit_adjustment,omitempty"`
	LimitAdoption           string         `json:"limit_adoption"`
	AnnualLegalLimit        float64        `json:"annual_legal_limit_msv"`
	NearLegalRatio          float64        `json:"near_legal_ratio"`
	ThresholdVersion        string         `json:"threshold_version"`
	RequiresManualReview    bool           `json:"requires_manual_review"`
	EscalationReason        string         `json:"escalation_reason"`
	BoundaryStatement       string         `json:"boundary_statement"`
}

func BuildArtifacts(snapshot Snapshot, summary PeriodSummary, decision Decision) (string, string, error) {
	snapshot.Controls = append([]string(nil), snapshot.Controls...)
	sort.Strings(snapshot.Controls)
	snapshot.IncludedExposureIDs = append([]uint(nil), summary.IncludedEntryIDs...)
	snapshot.ExcludedExposureIDs = append([]uint(nil), summary.ExcludedEntryIDs...)
	evidence := Evidence{
		PeriodStart: snapshot.PeriodStart, PeriodEnd: snapshot.PeriodEnd,
		VerifiedEntryCount: summary.VerifiedEntryCount, ExcludedEntryCount: summary.ExcludedEntryCount,
		CorrectedChainCount: summary.CorrectedChainCount,
		Formula:             "period_dose_msv = sum(verified exposure entries, including immutable reversals and replacements)",
		ProjectionFormula:   "projected_dose_msv = period_dose_msv + estimated_rate_msvh * planned_minutes / 60",
		AdministrativeLimit: snapshot.AdministrativeLimitMSV, BaseAdministrativeLimit: snapshot.BaseAdministrativeLimitMSV,
		LimitAdjustment: snapshot.LimitAdjustment, LimitAdoption: limitAdoption(snapshot),
		AnnualLegalLimit: snapshot.LegalLimitMSV,
		NearLegalRatio:   snapshot.NearLegalRatio, ThresholdVersion: snapshot.ThresholdVersion,
		RequiresManualReview: decision.RequiresManualReview, EscalationReason: decision.EscalationExplanation,
		BoundaryStatement: BoundaryStatement,
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return "", "", fmt.Errorf("encode assessment input snapshot: %w", err)
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return "", "", fmt.Errorf("encode assessment evidence: %w", err)
	}
	return string(snapshotJSON), string(evidenceJSON), nil
}

func RiskRank(value string) int {
	ranks := map[string]int{"within_admin": 1, "above_admin": 2, "near_legal": 3, "above_legal": 4, "invalid": 5}
	return ranks[value]
}

// limitAdoption explains in the frozen evidence whether an approved temporary
// adjustment or the worker profile limit governed the assessment.
func limitAdoption(snapshot Snapshot) string {
	if snapshot.LimitAdjustment == nil {
		return "No approved temporary adjustment covers the period end date; the worker profile administrative limit applies."
	}
	return fmt.Sprintf(
		"Approved temporary adjustment #%d covers the period end date; the administrative planning limit is %.4g mSv instead of the profile limit %.4g mSv for its validity window.",
		snapshot.LimitAdjustment.ID, snapshot.LimitAdjustment.AdjustedLimitMSV, snapshot.BaseAdministrativeLimitMSV,
	)
}
