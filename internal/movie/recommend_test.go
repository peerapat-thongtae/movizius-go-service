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

func TestRankByAffinityPreservesSetAndSize(t *testing.T) {
	candidates := []Movie{{MovieID: 1}, {MovieID: 2}, {MovieID: 3}, {MovieID: 4}}
	got := rankByAffinity(candidates, MovieAffinity{})
	if len(got) != len(candidates) {
		t.Fatalf("rankByAffinity() returned %d items, want %d", len(got), len(candidates))
	}
	seen := map[int64]bool{}
	for _, m := range got {
		seen[m.MovieID] = true
	}
	for _, c := range candidates {
		if !seen[c.MovieID] {
			t.Errorf("rankByAffinity() dropped candidate %d", c.MovieID)
		}
	}
}

func TestRankByAffinityOrdersByScoreDescending(t *testing.T) {
	liked := Movie{MovieID: 999, Genres: []int64{1}}
	disliked := Movie{MovieID: 1, Genres: []int64{2}}
	neutral := Movie{MovieID: 500}
	aff := MovieAffinity{Genres: map[int64]int{1: 100, 2: -100}}

	got := rankByAffinity([]Movie{disliked, neutral, liked}, aff)
	want := []int64{999, 500, 1}
	for i, id := range want {
		if got[i].MovieID != id {
			t.Fatalf("rankByAffinity() = %v, want order %v", movieIDs(got), want)
		}
	}
}

func TestRankByAffinityIsDeterministic(t *testing.T) {
	candidates := []Movie{{MovieID: 3}, {MovieID: 1}, {MovieID: 2}}
	aff := MovieAffinity{}

	first := rankByAffinity(candidates, aff)
	second := rankByAffinity(candidates, aff)
	for i := range first {
		if first[i].MovieID != second[i].MovieID {
			t.Fatalf("rankByAffinity() not deterministic: %v vs %v", movieIDs(first), movieIDs(second))
		}
	}
	// equal scores (all neutral) tie-break by ascending movie id.
	want := []int64{1, 2, 3}
	for i, id := range want {
		if first[i].MovieID != id {
			t.Errorf("rankByAffinity() tie-break = %v, want ascending id order %v", movieIDs(first), want)
		}
	}
}

func movieIDs(movies []Movie) []int64 {
	ids := make([]int64, len(movies))
	for i, m := range movies {
		ids[i] = m.MovieID
	}
	return ids
}
