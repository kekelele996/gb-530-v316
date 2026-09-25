package dto

import "time"

type CreateLimitAdjustmentRequest struct {
	EffectiveFrom    time.Time `json:"effective_from" validate:"required"`
	EffectiveTo      time.Time `json:"effective_to" validate:"required,gtfield=EffectiveFrom"`
	AdjustedLimitMSV float64   `json:"adjusted_limit_msv" validate:"required,gt=0,lte=1000"`
	Reason           string    `json:"reason" validate:"required,min=3,max=500"`
}

type ReviewLimitAdjustmentRequest struct {
	Decision string `json:"decision" validate:"required,oneof=approve reject"`
	Note     string `json:"note" validate:"max=500"`
}

type LimitAdjustmentResponse struct {
	ID               uint       `json:"id"`
	WorkerID         uint       `json:"worker_id"`
	WorkerCode       string     `json:"worker_code"`
	WorkerName       string     `json:"worker_name"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      time.Time  `json:"effective_to"`
	AdjustedLimitMSV float64    `json:"adjusted_limit_msv"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"`
	ReviewedBy       *uint      `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
	ReviewNote       string     `json:"review_note"`
	CreatedBy        uint       `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
}
