package dto

import "time"

type CreateTemporaryLimitAdjustmentRequest struct {
	WorkerID         uint      `json:"worker_id" validate:"required,gt=0"`
	EffectiveDate    time.Time `json:"effective_date" validate:"required"`
	ExpiryDate       time.Time `json:"expiry_date" validate:"required"`
	AdjustedLimitMSV float64   `json:"adjusted_limit_msv" validate:"required,gt=0,lte=1000"`
	Reason           string    `json:"reason" validate:"required,min=3,max=1000"`
}

type ReviewTemporaryLimitAdjustmentRequest struct {
	Decision        string `json:"decision" validate:"required,oneof=approve reject"`
	RejectionReason string `json:"rejection_reason" validate:"max=1000"`
}

type TemporaryLimitAdjustmentResponse struct {
	ID               uint       `json:"id"`
	WorkerID         uint       `json:"worker_id"`
	WorkerCode       string     `json:"worker_code"`
	WorkerName       string     `json:"worker_name"`
	Status           string     `json:"status"`
	EffectiveDate    time.Time  `json:"effective_date"`
	ExpiryDate       time.Time  `json:"expiry_date"`
	AdjustedLimitMSV float64    `json:"adjusted_limit_msv"`
	Reason           string     `json:"reason"`
	CreatedBy        uint       `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	ReviewedBy       *uint      `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
	RejectionReason  string     `json:"rejection_reason"`
}
