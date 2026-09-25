package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"radiation-dose-budget-control/backend/internal/model"
)

type LimitAdjustmentRepository struct{ db *gorm.DB }

func NewLimitAdjustmentRepository(db *gorm.DB) *LimitAdjustmentRepository {
	return &LimitAdjustmentRepository{db: db}
}

func (repository *LimitAdjustmentRepository) WithDB(db *gorm.DB) *LimitAdjustmentRepository {
	return &LimitAdjustmentRepository{db: db}
}

func (repository *LimitAdjustmentRepository) Create(adjustment *model.LimitAdjustment) error {
	if err := repository.db.Create(adjustment).Error; err != nil {
		return fmt.Errorf("create limit adjustment: %w", err)
	}
	return nil
}

func (repository *LimitAdjustmentRepository) Find(id uint) (model.LimitAdjustment, error) {
	var adjustment model.LimitAdjustment
	if err := repository.db.First(&adjustment, id).Error; err != nil {
		return adjustment, fmt.Errorf("find limit adjustment: %w", err)
	}
	return adjustment, nil
}

func (repository *LimitAdjustmentRepository) FindForUpdate(id uint) (model.LimitAdjustment, error) {
	var adjustment model.LimitAdjustment
	if err := repository.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&adjustment, id).Error; err != nil {
		return adjustment, fmt.Errorf("lock limit adjustment: %w", err)
	}
	return adjustment, nil
}

func (repository *LimitAdjustmentRepository) ListForWorker(workerID uint) ([]model.LimitAdjustment, error) {
	var adjustments []model.LimitAdjustment
	err := repository.db.Where("worker_id = ?", workerID).
		Order("effective_from DESC, id DESC").Find(&adjustments).Error
	if err != nil {
		return nil, fmt.Errorf("list limit adjustments: %w", err)
	}
	return adjustments, nil
}

// HasOverlappingActive reports whether a pending or approved adjustment already
// overlaps the half-open window [from, to) for the worker.
func (repository *LimitAdjustmentRepository) HasOverlappingActive(workerID uint, from, to time.Time) (bool, error) {
	var count int64
	err := repository.db.Model(&model.LimitAdjustment{}).
		Where("worker_id = ? AND status IN ? AND effective_from < ? AND effective_to > ?",
			workerID, []string{"pending", "approved"}, to, from).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count overlapping limit adjustments: %w", err)
	}
	return count > 0, nil
}

// ApprovedCovering returns the approved adjustment whose half-open window
// contains instant at; it returns gorm.ErrRecordNotFound when none applies.
func (repository *LimitAdjustmentRepository) ApprovedCovering(workerID uint, at time.Time) (model.LimitAdjustment, error) {
	var adjustment model.LimitAdjustment
	err := repository.db.
		Where("worker_id = ? AND status = ? AND effective_from <= ? AND effective_to > ?",
			workerID, "approved", at, at).
		Order("effective_from DESC, id DESC").First(&adjustment).Error
	if err != nil {
		return adjustment, fmt.Errorf("find approved limit adjustment: %w", err)
	}
	return adjustment, nil
}

func (repository *LimitAdjustmentRepository) Review(id uint, fromStatus, toStatus string, reviewerID uint, reviewedAt time.Time, note string) error {
	result := repository.db.Model(&model.LimitAdjustment{}).
		Where("id = ? AND status = ?", id, fromStatus).
		Updates(map[string]any{
			"status": toStatus, "reviewed_by": reviewerID, "reviewed_at": reviewedAt,
			"review_note": note, "updated_at": reviewedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("review limit adjustment: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("review limit adjustment: %w", ErrStateConflict)
	}
	return nil
}
