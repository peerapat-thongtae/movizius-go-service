package recommendation

import (
	"math"
	"testing"
	"time"
)

func floatsClose(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestRecencyWeight(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name         string
		referenceAt  time.Time
		halfLifeDays float64
		want         float64
	}{
		{"zero days since", now, 90, 1.0},
		{"one half-life", now.Add(-90 * 24 * time.Hour), 90, 0.5},
		{"two half-lives", now.Add(-180 * 24 * time.Hour), 90, 0.25},
		{"future reference clamps to zero days", now.Add(24 * time.Hour), 90, 1.0},
		{"non-positive half-life is a no-op", now.Add(-90 * 24 * time.Hour), 0, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RecencyWeight(now, tc.referenceAt, tc.halfLifeDays)
			if !floatsClose(got, tc.want, 1e-9) {
				t.Errorf("RecencyWeight() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRatingSignal(t *testing.T) {
	r := func(v float64) *float64 { return &v }
	cases := []struct {
		name          string
		rating        *float64
		completionPct float64
		want          float64
	}{
		{"rating 10 -> +1", r(10), 0, 1.0},
		{"rating 0 -> -1", r(0), 0, -1.0},
		{"rating 5 -> neutral", r(5), 0, 0.0},
		{"nil rating, completion 1 -> +1", nil, 1.0, 1.0},
		{"nil rating, completion 0 -> -1", nil, 0.0, -1.0},
		{"nil rating, completion 0.5 -> neutral", nil, 0.5, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RatingSignal(tc.rating, tc.completionPct)
			if !floatsClose(got, tc.want, 1e-9) {
				t.Errorf("RatingSignal() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRewatchBonus(t *testing.T) {
	if got := RewatchBonus(0, 0.3); got != 1 {
		t.Errorf("RewatchBonus(0, ...) = %v, want 1", got)
	}
	b1 := RewatchBonus(1, 0.3)
	b2 := RewatchBonus(2, 0.3)
	if !(b1 > 1) {
		t.Errorf("RewatchBonus(1, ...) = %v, want > 1", b1)
	}
	if !(b2 > b1) {
		t.Errorf("RewatchBonus should be monotonically increasing: b1=%v b2=%v", b1, b2)
	}
}

func TestContribution(t *testing.T) {
	now := time.Now().UTC()
	r := func(v float64) *float64 { return &v }

	positive := Contribution(EventInput{
		Now: now, ReferenceAt: now, CompletionPct: 1.0, Rating: r(10),
		HalfLifeDays: 90, RewatchBonusK: 0.3,
	})
	if positive <= 0 {
		t.Errorf("expected positive contribution for a top rating, got %v", positive)
	}

	negative := Contribution(EventInput{
		Now: now, ReferenceAt: now, CompletionPct: 1.0, Rating: r(0),
		HalfLifeDays: 90, RewatchBonusK: 0.3,
	})
	if negative >= 0 {
		t.Errorf("expected negative contribution for a bottom rating, got %v", negative)
	}

	neutral := Contribution(EventInput{
		Now: now, ReferenceAt: now, CompletionPct: 1.0, Rating: r(5),
		HalfLifeDays: 90, RewatchBonusK: 0.3,
	})
	if !floatsClose(neutral, 0, 1e-9) {
		t.Errorf("expected ~0 contribution for a neutral rating, got %v", neutral)
	}
}
