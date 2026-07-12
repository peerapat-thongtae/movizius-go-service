package movie

import "testing"

func TestAffinityScoreNeutralWhenNoMatch(t *testing.T) {
	m := MovieResponse{Genres: []Genre{{ID: 1}}}
	if got := affinityScore(m, MovieAffinity{}); got != 0 {
		t.Errorf("affinityScore() = %v, want 0 for empty affinity", got)
	}
}

func TestAffinityScoreAveragesMatches(t *testing.T) {
	m := MovieResponse{
		Genres: []Genre{{ID: 1}, {ID: 2}},
	}
	aff := MovieAffinity{Genres: map[int64]int{1: 80, 2: 40}}
	got := affinityScore(m, aff)
	want := 60.0
	if got != want {
		t.Errorf("affinityScore() = %v, want %v", got, want)
	}
}

func TestWeightFromScore(t *testing.T) {
	if got := weightFromScore(0); got != 1 {
		t.Errorf("weightFromScore(0) = %v, want 1", got)
	}
	if got := weightFromScore(50); got != 2 {
		t.Errorf("weightFromScore(50) = %v, want 2", got)
	}
	if w := weightFromScore(-50); !(w > 0 && w < 1) {
		t.Errorf("weightFromScore(-50) = %v, want in (0,1)", w)
	}
}

func TestWeightedSampleNeutralIsUnbiasedSize(t *testing.T) {
	pool := []MovieResponse{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}
	got := weightedSample(pool, MovieAffinity{}, 3)
	if len(got) != 3 {
		t.Fatalf("weightedSample() returned %d items, want 3", len(got))
	}
	seen := map[int64]bool{}
	for _, m := range got {
		if seen[m.ID] {
			t.Errorf("weightedSample() returned duplicate id %d", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestWeightedSampleReturnsAllWhenPoolSmallerThanTotal(t *testing.T) {
	pool := []MovieResponse{{ID: 1}, {ID: 2}}
	got := weightedSample(pool, MovieAffinity{}, 5)
	if len(got) != 2 {
		t.Errorf("weightedSample() returned %d items, want 2 (pool size)", len(got))
	}
}

func TestWeightedSampleBiasesTowardHigherAffinity(t *testing.T) {
	// Build a large pool where one candidate has a strongly liked genre and
	// the rest are neutral; over many trials it should be picked first far
	// more often than uniform chance would predict.
	liked := MovieResponse{ID: 999, Genres: []Genre{{ID: 1}}}
	aff := MovieAffinity{Genres: map[int64]int{1: 100}}

	firstPickCount := 0
	trials := 200
	for i := 0; i < trials; i++ {
		pool := []MovieResponse{liked}
		for j := int64(0); j < 19; j++ {
			pool = append(pool, MovieResponse{ID: j})
		}
		got := weightedSample(pool, aff, 1)
		if len(got) == 1 && got[0].ID == liked.ID {
			firstPickCount++
		}
	}
	// Uniform chance would be ~1/20 = 5% (~10 out of 200); a strong positive
	// affinity should push this notably above that.
	if firstPickCount < trials/10 {
		t.Errorf("liked candidate picked first %d/%d times, expected notably more than uniform (~%d)", firstPickCount, trials, trials/20)
	}
}
