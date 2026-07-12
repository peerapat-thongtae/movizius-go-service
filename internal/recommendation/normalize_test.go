package recommendation

import (
	"math"
	"testing"
)

func TestScore(t *testing.T) {
	cases := []struct {
		name   string
		rawSum float64
		count  int
		want   int
	}{
		{"zero count", 5, 0, 0},
		{"neutral", 0, 3, 0},
		{"large positive saturates near 100", 1000, 1, 100},
		{"large negative saturates near -100", -1000, 1, -100},
		{"negative rawSum yields negative score", -0.5, 2, int(math.Round(math.Tanh(-0.25) * 100))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Score(tc.rawSum, tc.count)
			if got != tc.want {
				t.Errorf("Score(%v, %v) = %v, want %v", tc.rawSum, tc.count, got, tc.want)
			}
		})
	}
}

func TestShouldPrune(t *testing.T) {
	cases := []struct {
		name        string
		entry       BucketEntry
		minCount    int
		maxAbsScore int
		want        bool
	}{
		{"low count, low score -> prune", BucketEntry{Score: 5, Count: 1}, 2, 10, true},
		{"low count, high score -> keep", BucketEntry{Score: 50, Count: 1}, 2, 10, false},
		{"high count, low score -> keep", BucketEntry{Score: 5, Count: 5}, 2, 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldPrune(tc.entry, tc.minCount, tc.maxAbsScore)
			if got != tc.want {
				t.Errorf("ShouldPrune(%+v) = %v, want %v", tc.entry, got, tc.want)
			}
		})
	}
}

func TestPruneAndCap(t *testing.T) {
	b := Bucket{
		"1": {Score: 5, Count: 1},   // pruned: low count + low score
		"2": {Score: 90, Count: 10}, // kept: high rank
		"3": {Score: 20, Count: 3},  // kept: moderate rank
		"4": {Score: 15, Count: 1},  // kept: high score alone survives prune, but capped out if topN small
	}

	pruned := PruneAndCap(b, 2, 10, 100)
	if _, ok := pruned["1"]; ok {
		t.Errorf("expected entity 1 to be pruned, got %+v", pruned["1"])
	}
	if len(pruned) != 3 {
		t.Errorf("expected 3 surviving entities after prune, got %d: %+v", len(pruned), pruned)
	}

	capped := PruneAndCap(b, 2, 10, 1)
	if len(capped) != 1 {
		t.Errorf("expected exactly 1 entity after cap, got %d: %+v", len(capped), capped)
	}
	if _, ok := capped["2"]; !ok {
		t.Errorf("expected entity 2 (highest rank) to survive the cap, got %+v", capped)
	}
}
