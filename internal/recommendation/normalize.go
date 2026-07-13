package recommendation

import (
	"math"
	"sort"
)

// Score converts an accumulated (rawSum, weightSum) pair into the public
// -100..100 integer score via a tanh squash (smooth bound, no hard clipping).
// weightSum (recency-weighted evidence), not a raw event count, is the
// denominator — this makes avg a true weighted average of each event's
// signal, so a long tail of stale/decayed events doesn't crush the score
// toward zero the way dividing by a flat count would.
func Score(rawSum, weightSum float64) int {
	if weightSum == 0 {
		return 0
	}
	avg := rawSum / weightSum
	return int(math.Round(math.Tanh(avg) * 100))
}

// ShouldPrune reports whether a bucket entry is stale/noise and should be
// dropped: low provenance (count) and near-zero score.
func ShouldPrune(e BucketEntry, minCount, maxAbsScore int) bool {
	return e.Count < minCount && abs(e.Score) < maxAbsScore
}

// PruneAndCap drops stale entries (per ShouldPrune) then caps the remaining
// bucket to the top N entities by |score|*log(count+1), preventing
// unbounded document growth.
func PruneAndCap(b Bucket, minCount, maxAbsScore, topN int) Bucket {
	if len(b) == 0 {
		return b
	}

	type ranked struct {
		id    string
		entry BucketEntry
		rank  float64
	}
	kept := make([]ranked, 0, len(b))
	for id, entry := range b {
		if ShouldPrune(entry, minCount, maxAbsScore) {
			continue
		}
		rank := float64(abs(entry.Score)) * math.Log(float64(entry.Count)+1)
		kept = append(kept, ranked{id: id, entry: entry, rank: rank})
	}

	sort.Slice(kept, func(i, j int) bool { return kept[i].rank > kept[j].rank })
	if topN > 0 && len(kept) > topN {
		kept = kept[:topN]
	}

	out := make(Bucket, len(kept))
	for _, k := range kept {
		out[k.id] = k.entry
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
