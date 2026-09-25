package model

import "time"

// TemporaryLimitAdjustment is a planner-proposed, RPO-approved temporary
// override of a worker's administrative planning limit. It never changes the
// worker profile itself; assessments opt in by date when an approved window
// covers the period end. Rejected proposals are retained for audit.
type TemporaryLimitAdjustment struct {
	ID               uint      `gorm:"primaryKey"`
	WorkerID         uint      `gorm:"index;not null"`
	Status           string    `gorm:"size:24;index;not null"`
	EffectiveDate    time.Time `gorm:"index;not null"`
	ExpiryDate       time.Time `gorm:"index;not null"`
	AdjustedLimitMSV float64   `gorm:"not null"`
	Reason           string    `gorm:"type:text;not null"`
	CreatedBy        uint      `gorm:"index;not null"`
	CreatedAt        time.Time `gorm:"not null"`
	ReviewedBy       *uint     `gorm:"index"`
	ReviewedAt       *time.Time
	RejectionReason  string `gorm:"type:text;not null;default:''"`
}
