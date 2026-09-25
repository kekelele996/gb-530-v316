package model

import "time"

type LimitAdjustment struct {
	ID               uint      `gorm:"primaryKey"`
	WorkerID         uint      `gorm:"index;not null"`
	EffectiveFrom    time.Time `gorm:"index;not null"`
	EffectiveTo      time.Time `gorm:"not null"`
	AdjustedLimitMSV float64   `gorm:"not null"`
	Reason           string    `gorm:"size:500;not null"`
	Status           string    `gorm:"size:24;index;not null"`
	ReviewedBy       *uint     `gorm:"index"`
	ReviewedAt       *time.Time
	ReviewNote       string    `gorm:"size:500;not null;default:''"`
	CreatedBy        uint      `gorm:"index;not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}
