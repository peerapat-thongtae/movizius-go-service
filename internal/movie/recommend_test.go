package movie

import (
	"sort"
	"testing"
)

func TestPositiveScoreIDs(t *testing.T) {
	scores := map[int64]int{1: 10, 2: -5, 3: 0, 4: 100}
	got := positiveScoreIDs(scores)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	want := []int64{1, 4}
	if len(got) != len(want) {
		t.Fatalf("positiveScoreIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("positiveScoreIDs() = %v, want %v", got, want)
		}
	}
}

func TestCandidateAffinityScoreNeutralWhenNoMatch(t *testing.T) {
	m := Movie{Genres: []int64{1}}
	if got := candidateAffinityScore(m, MovieAffinity{}); got != 0 {
		t.Errorf("candidateAffinityScore() = %v, want 0 for empty affinity", got)
	}
}

func TestCandidateAffinityScoreAveragesMatches(t *testing.T) {
	director := int64(50)
	collection := int64(60)
	m := Movie{
		Genres:              []int64{1, 2},
		Keywords:            []int64{10},
		CastIDs:             []int64{20},
		DirectorID:          &director,
		CollectionID:        &collection,
		ProductionCompanies: []int64{30},
	}
	aff := MovieAffinity{
		Genres:              map[int64]int{1: 80, 2: 40},
		Keywords:            map[int64]int{10: 60},
		Actors:              map[int64]int{20: 100},
		Directors:           map[int64]int{50: -20},
		Collections:         map[int64]int{60: 0},
		ProductionCompanies: map[int64]int{30: 20},
	}
	// matches: 80, 40, 60, 100, -20, 0, 20 -> sum 280, n 7 -> avg 40
	got := candidateAffinityScore(m, aff)
	want := 280.0 / 7.0
	if got != want {
		t.Errorf("candidateAffinityScore() = %v, want %v", got, want)
	}
}

func TestCandidateAffinityScoreIgnoresUnmatchedEntities(t *testing.T) {
	m := Movie{Genres: []int64{1, 2, 3}}
	aff := MovieAffinity{Genres: map[int64]int{2: 50}}
	got := candidateAffinityScore(m, aff)
	if got != 50 {
		t.Errorf("candidateAffinityScore() = %v, want 50 (only genre 2 matches)", got)
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

func TestWeightedRankPreservesSetAndSize(t *testing.T) {
	candidates := []Movie{{MovieID: 1}, {MovieID: 2}, {MovieID: 3}, {MovieID: 4}}
	got := weightedRank(candidates, MovieAffinity{})
	if len(got) != len(candidates) {
		t.Fatalf("weightedRank() returned %d items, want %d", len(got), len(candidates))
	}
	seen := map[int64]bool{}
	for _, m := range got {
		seen[m.MovieID] = true
	}
	for _, c := range candidates {
		if !seen[c.MovieID] {
			t.Errorf("weightedRank() dropped candidate %d", c.MovieID)
		}
	}
}

func TestWeightedRankBiasesTowardHigherAffinity(t *testing.T) {
	liked := Movie{MovieID: 999, Genres: []int64{1}}
	aff := MovieAffinity{Genres: map[int64]int{1: 100}}

	firstPickCount := 0
	trials := 200
	for i := 0; i < trials; i++ {
		candidates := []Movie{liked}
		for j := int64(0); j < 19; j++ {
			candidates = append(candidates, Movie{MovieID: j})
		}
		got := weightedRank(candidates, aff)
		if len(got) > 0 && got[0].MovieID == liked.MovieID {
			firstPickCount++
		}
	}
	// Uniform chance would be ~1/20 = 5% (~10 out of 200); a strong positive
	// affinity should push this notably above that.
	if firstPickCount < trials/10 {
		t.Errorf("liked candidate ranked first %d/%d times, expected notably more than uniform (~%d)", firstPickCount, trials, trials/20)
	}
}
