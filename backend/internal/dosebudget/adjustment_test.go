package dosebudget

import (
	"testing"
	"time"
)

func TestAdjustmentWindowUsesHalfOpenBoundary(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	window, err := NewAdjustmentWindow(from, to)
	if err != nil {
		t.Fatalf("NewAdjustmentWindow returned error: %v", err)
	}
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"effective start included", from, true},
		{"inside included", from.Add(15 * 24 * time.Hour), true},
		{"last nanosecond included", to.Add(-time.Nanosecond), true},
		{"expiry excluded", to, false},
		{"before start excluded", from.Add(-time.Nanosecond), false},
		{"non-UTC instant normalized", time.Date(2026, 9, 1, 2, 0, 0, 0, time.FixedZone("UTC+2", 2*3600)), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := window.Covers(test.at); got != test.want {
				t.Fatalf("Covers(%s) = %v, want %v", test.at, got, test.want)
			}
		})
	}
}

func TestNewAdjustmentWindowRejectsInvalidRanges(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, to := range []time.Time{from, from.Add(-time.Second)} {
		if _, err := NewAdjustmentWindow(from, to); err == nil {
			t.Fatalf("NewAdjustmentWindow(%s, %s) unexpectedly succeeded", from, to)
		}
	}
	if _, err := NewAdjustmentWindow(time.Time{}, from); err == nil {
		t.Fatal("NewAdjustmentWindow accepted a zero effective_from")
	}
}

func TestAdjustmentWindowOverlap(t *testing.T) {
	base, err := NewAdjustmentWindow(
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewAdjustmentWindow returned error: %v", err)
	}
	window := func(startDay, endDay int) AdjustmentWindow {
		candidate, err := NewAdjustmentWindow(
			time.Date(2026, 9, startDay, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, endDay, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("NewAdjustmentWindow returned error: %v", err)
		}
		return candidate
	}
	tests := []struct {
		name  string
		other AdjustmentWindow
		want  bool
	}{
		{"before and touching start", window(1, 10), false},
		{"ending inside", window(1, 15), true},
		{"fully inside", window(12, 15), true},
		{"fully covering", window(1, 28), true},
		{"starting inside", window(15, 28), true},
		{"touching end", window(20, 28), false},
		{"after", window(22, 28), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := base.Overlaps(test.other); got != test.want {
				t.Fatalf("Overlaps = %v, want %v", got, test.want)
			}
			if got := test.other.Overlaps(base); got != test.want {
				t.Fatalf("symmetric Overlaps = %v, want %v", got, test.want)
			}
		})
	}
}
