package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/model"
)

type TemporaryLimitAdjustmentRepository struct{ db *gorm.DB }

func NewTemporaryLimitAdjustmentRepository(db *gorm.DB) *TemporaryLimitAdjustmentRepository {
	return &TemporaryLimitAdjustmentRepository{db: db}
}

func (repository *TemporaryLimitAdjustmentRepository) WithDB(db *gorm.DB) *TemporaryLimitAdjustmentRepository {
	return &TemporaryLimitAdjustmentRepository{db: db}
}

func (repository *TemporaryLimitAdjustmentRepository) Create(adjustment *model.TemporaryLimitAdjustment) error {
	if err := repository.db.Create(adjustment).Error; err != nil {
		return fmt.Errorf("create temporary limit adjustment: %w", err)
	}
	return nil
}

func (repository *TemporaryLimitAdjustmentRepository) Find(id uint) (model.TemporaryLimitAdjustment, error) {
	var adjustment model.TemporaryLimitAdjustment
	if err := repository.db.First(&adjustment, id).Error; err != nil {
		return adjustment, fmt.Errorf("find temporary limit adjustment: %w", err)
	}
	return adjustment, nil
}

func (repository *TemporaryLimitAdjustmentRepository) FindForUpdate(id uint) (model.TemporaryLimitAdjustment, error) {
	var adjustment model.TemporaryLimitAdjustment
	if err := repository.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&adjustment, id).Error; err != nil {
		return adjustment, fmt.Errorf("lock temporary limit adjustment: %w", err)
	}
	return adjustment, nil
}

func (repository *TemporaryLimitAdjustmentRepository) List(page, pageSize int, workerID uint, status string) ([]model.TemporaryLimitAdjustment, int64, error) {
	query := repository.db.Model(&model.TemporaryLimitAdjustment{})
	if workerID > 0 {
		query = query.Where("worker_id = ?", workerID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count temporary limit adjustments: %w", err)
	}
	var adjustments []model.TemporaryLimitAdjustment
	if err := query.Order("effective_date DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&adjustments).Error; err != nil {
		return nil, 0, fmt.Errorf("list temporary limit adjustments: %w", err)
	}
	return adjustments, total, nil
}

// ActiveForWorker returns pending or approved adjustments whose [effective, expiry)
// window overlaps the given half-open window, so a new proposal can be rejected
// when it would duplicate an unresolved or already accepted window.
func (repository *TemporaryLimitAdjustmentRepository) Overlapping(workerID uint, start, end time.Time) ([]model.TemporaryLimitAdjustment, error) {
	var adjustments []model.TemporaryLimitAdjustment
	err := repository.db.
		Where("worker_id = ?", workerID).
		Where("status IN ?", []string{constants.LimitAdjustmentStatusPending, constants.LimitAdjustmentStatusApproved}).
		Where("effective_date < ? AND expiry_date > ?", end, start).
		Order("effective_date ASC, id ASC").
		Find(&adjustments).Error
	if err != nil {
		return nil, fmt.Errorf("find overlapping temporary limit adjustments: %w", err)
	}
	return adjustments, nil
}

// EffectiveAt returns the approved adjustment that covers the given instant.
// Overlapping approved windows cannot be created, so at most one row matches.
func (repository *TemporaryLimitAdjustmentRepository) EffectiveAt(workerID uint, at time.Time) (model.TemporaryLimitAdjustment, error) {
	var adjustment model.TemporaryLimitAdjustment
	err := repository.db.
		Where("worker_id = ?", workerID).
		Where("status = ?", constants.LimitAdjustmentStatusApproved).
		Where("effective_date <= ? AND expiry_date > ?", at, at).
		Order("effective_date DESC, id DESC").
		First(&adjustment).Error
	if err != nil {
		return adjustment, fmt.Errorf("find effective temporary limit adjustment: %w", err)
	}
	return adjustment, nil
}

func (repository *TemporaryLimitAdjustmentRepository) Review(id uint, from, to string, reviewerID uint, reviewedAt time.Time, rejectionReason string) error {
	result := repository.db.Model(&model.TemporaryLimitAdjustment{}).
		Where("id = ? AND status = ?", id, from).
		Updates(map[string]any{
			"status": to, "reviewed_by": reviewerID, "reviewed_at": reviewedAt,
			"rejection_reason": rejectionReason,
		})
	if result.Error != nil {
		return fmt.Errorf("review temporary limit adjustment: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("review temporary limit adjustment: %w", ErrStateConflict)
	}
	return nil
}
