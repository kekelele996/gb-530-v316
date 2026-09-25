package dosebudget

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidAdjustmentWindow = errors.New("invalid adjustment window")

// AdjustmentWindow is the half-open validity window [From, To) of a temporary
// administrative limit adjustment. It mirrors the assessment period convention.
type AdjustmentWindow struct {
	From time.Time
	To   time.Time
}

func NewAdjustmentWindow(from, to time.Time) (AdjustmentWindow, error) {
	from = from.UTC()
	to = to.UTC()
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return AdjustmentWindow{}, fmt.Errorf("%w: effective_to must be after effective_from", ErrInvalidAdjustmentWindow)
	}
	return AdjustmentWindow{From: from, To: to}, nil
}

// Covers reports whether instant value falls inside the half-open window [From, To).
func (window AdjustmentWindow) Covers(value time.Time) bool {
	value = value.UTC()
	return !value.Before(window.From) && value.Before(window.To)
}

// Overlaps reports whether two half-open windows share any instant.
func (window AdjustmentWindow) Overlaps(other AdjustmentWindow) bool {
	return window.From.Before(other.To) && other.From.Before(window.To)
}
