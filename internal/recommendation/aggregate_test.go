package recommendation

import "testing"

func TestDeltasZeroWeight(t *testing.T) {
	refs := EntityRefs{Genres: []int64{1, 2}}
	if got := Deltas(1.0, 0, refs, Multipliers{}); got != nil {
		t.Errorf("Deltas(1.0, 0, ...) = %v, want nil", got)
	}
}

func TestDeltasNeutralSignalStillCounts(t *testing.T) {
	refs := EntityRefs{Genres: []int64{1}}
	deltas := Deltas(0, 1.0, refs, Multipliers{})
	if len(deltas) != 1 {
		t.Fatalf("Deltas(0, 1.0, ...) = %v, want 1 delta (weight nonzero even though contribution is 0)", deltas)
	}
	if deltas[0].RawSumDelta != 0 {
		t.Errorf("RawSumDelta = %v, want 0", deltas[0].RawSumDelta)
	}
	if deltas[0].WeightDelta != 1.0 {
		t.Errorf("WeightDelta = %v, want 1.0", deltas[0].WeightDelta)
	}
}

func TestDeltasEmptyRefs(t *testing.T) {
	if got := Deltas(1.0, 1.0, EntityRefs{}, Multipliers{}); got != nil {
		t.Errorf("Deltas(1.0, 1.0, EntityRefs{}, ...) = %v, want nil", got)
	}
}

func TestDeltasSkipsActorsForAnimatedTitles(t *testing.T) {
	refs := EntityRefs{
		Genres:  []int64{AnimationGenreID, 1},
		CastIDs: []int64{10, 11},
	}
	deltas := Deltas(2.0, 1.0, refs, Multipliers{LeadActor: 1.2, Actor: 1.0})

	for _, d := range deltas {
		if d.Bucket == BucketActors {
			t.Errorf("expected no actor deltas for an animated title, got %+v", d)
		}
	}
	foundGenre := false
	for _, d := range deltas {
		if d.Bucket == BucketGenres && d.EntityID == AnimationGenreID {
			foundGenre = true
		}
	}
	if !foundGenre {
		t.Error("expected the animation genre itself to still be recorded")
	}
}

func TestDeltasKeepsActorsForNonAnimatedTitles(t *testing.T) {
	refs := EntityRefs{
		Genres:  []int64{28}, // Action, not animation
		CastIDs: []int64{10},
	}
	deltas := Deltas(2.0, 1.0, refs, Multipliers{LeadActor: 1.2, Actor: 1.0})

	found := false
	for _, d := range deltas {
		if d.Bucket == BucketActors && d.EntityID == 10 {
			found = true
		}
	}
	if !found {
		t.Error("expected actor delta to be recorded for a non-animated title")
	}
}

func TestDeltasMultipliers(t *testing.T) {
	director := int64(500)
	collection := int64(600)
	refs := EntityRefs{
		Genres:              []int64{1},
		Keywords:            []int64{2},
		CastIDs:             []int64{10, 11, 12},
		DirectorID:          &director,
		CreatorIDs:          []int64{700},
		CollectionID:        &collection,
		ProductionCompanies: []int64{20},
	}
	m := Multipliers{LeadActor: 1.2, Actor: 1.0, Director: 1.2, Creator: 1.2}
	weight := 1.5
	deltas := Deltas(2.0, weight, refs, m)

	find := func(bucket string, id int64) (BucketDelta, bool) {
		for _, d := range deltas {
			if d.Bucket == bucket && d.EntityID == id {
				return d, true
			}
		}
		return BucketDelta{}, false
	}

	if d, ok := find(BucketGenres, 1); !ok || d.RawSumDelta != 2.0*DefaultMultiplier {
		t.Errorf("genre delta = %+v, ok=%v", d, ok)
	}
	if d, ok := find(BucketKeywords, 2); !ok || d.RawSumDelta != 2.0*DefaultMultiplier {
		t.Errorf("keyword delta = %+v, ok=%v", d, ok)
	}
	if d, ok := find(BucketActors, 10); !ok || d.RawSumDelta != 2.0*m.LeadActor {
		t.Errorf("lead actor (index 0) delta = %+v, ok=%v, want lead multiplier", d, ok)
	}
	if d, ok := find(BucketActors, 11); !ok || d.RawSumDelta != 2.0*m.Actor {
		t.Errorf("non-lead actor delta = %+v, ok=%v, want actor multiplier", d, ok)
	}
	if d, ok := find(BucketDirectors, director); !ok || d.RawSumDelta != 2.0*m.Director {
		t.Errorf("director delta = %+v, ok=%v", d, ok)
	}
	if d, ok := find(BucketCreators, 700); !ok || d.RawSumDelta != 2.0*m.Creator {
		t.Errorf("creator delta = %+v, ok=%v", d, ok)
	}
	if d, ok := find(BucketCollections, collection); !ok || d.RawSumDelta != 2.0*DefaultMultiplier {
		t.Errorf("collection delta = %+v, ok=%v", d, ok)
	}
	if d, ok := find(BucketProductionCompanies, 20); !ok || d.RawSumDelta != 2.0*DefaultMultiplier {
		t.Errorf("production company delta = %+v, ok=%v", d, ok)
	}

	// WeightDelta must NOT be scaled by the bucket multiplier — every delta
	// carries the same raw weight, otherwise mult would cancel out of
	// RawSum/WeightSum entirely.
	for _, d := range deltas {
		if d.WeightDelta != weight {
			t.Errorf("bucket %s entity %d: WeightDelta = %v, want unscaled weight %v", d.Bucket, d.EntityID, d.WeightDelta, weight)
		}
	}
}
