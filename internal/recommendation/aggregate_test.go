package recommendation

import "testing"

func TestDeltasZeroContribution(t *testing.T) {
	refs := EntityRefs{Genres: []int64{1, 2}}
	if got := Deltas(0, refs, Multipliers{}); got != nil {
		t.Errorf("Deltas(0, ...) = %v, want nil", got)
	}
}

func TestDeltasEmptyRefs(t *testing.T) {
	if got := Deltas(1.0, EntityRefs{}, Multipliers{}); got != nil {
		t.Errorf("Deltas(1.0, EntityRefs{}, ...) = %v, want nil", got)
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
	deltas := Deltas(2.0, refs, m)

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
}
