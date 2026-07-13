package recommendation

import (
	"math"
	"time"
)

// EventInput is the minimal signal needed to compute a watch-event
// contribution, built by service.go from a MovieUser/TVUser doc plus its
// cached Movie/TV metadata.
type EventInput struct {
	Now           time.Time
	ReferenceAt   time.Time // "when watched" anchor for recency decay
	CompletionPct float64   // 0..1
	Rating        *float64  // 0..10 scale, nil if not rated
	RewatchCount  int       // always 0 today — see RewatchBonus doc
	HalfLifeDays  float64
	RewatchBonusK float64
}

// RecencyWeight computes exp(-ln2 * daysSince / halfLifeDays), i.e. an
// exponential decay that halves every halfLifeDays days since referenceAt.
func RecencyWeight(now, referenceAt time.Time, halfLifeDays float64) float64 {
	if halfLifeDays <= 0 {
		return 1
	}
	daysSince := now.Sub(referenceAt).Hours() / 24
	if daysSince < 0 {
		daysSince = 0
	}
	return math.Exp(-math.Ln2 * daysSince / halfLifeDays)
}

// RatingSignal maps an explicit rating (0..10 scale, 5 = neutral) or,
// absent that, a completion percentage (0..1, 0.5 = neutral) to a signed
// -1..+1 preference signal. This mirrors the spec's 1..5 rating mapping
// ((rating-3)/2) generalized to this codebase's 0..10 rating scale.
func RatingSignal(rating *float64, completionPct float64) float64 {
	if rating != nil {
		return (*rating - 5) / 5
	}
	return (completionPct - 0.5) * 2
}

// RewatchBonus computes 1 + ln(1+rewatchCount)*k. rewatchCount is always 0
// in the current data model (no reliable rewatch counter exists), making
// this a no-op multiplier of 1 today; it stays a real function so a future
// rewatch signal can be wired in without changing the contribution formula.
func RewatchBonus(rewatchCount int, k float64) float64 {
	if rewatchCount <= 0 {
		return 1
	}
	return 1 + math.Log(1+float64(rewatchCount))*k
}

// Weight computes the unsigned "how much evidence does this event carry"
// magnitude: recencyWeight * completionPct * rewatchBonus. Always >= 0. Used
// as the denominator when averaging signals into a bucket score, so recent/
// complete events count more than stale ones regardless of how many stale
// events accumulate.
func Weight(in EventInput) float64 {
	recency := RecencyWeight(in.Now, in.ReferenceAt, in.HalfLifeDays)
	bonus := RewatchBonus(in.RewatchCount, in.RewatchBonusK)
	return recency * in.CompletionPct * bonus
}

// Contribution computes the final signed contribution for one watch event:
// weight * ratingSignal.
func Contribution(in EventInput) float64 {
	return Weight(in) * RatingSignal(in.Rating, in.CompletionPct)
}
