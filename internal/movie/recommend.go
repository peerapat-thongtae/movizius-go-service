package movie

import "sort"

// candidateAffinityScore averages the user's recommendation-profile scores
// across every entity of m that appears in aff (genres, keywords, cast,
// director, collection, production companies). Operates directly on Movie's
// flat []int64/*int64 id fields (populated from TMDB at sync time), so no
// TMDB fetch is needed to score a candidate. Returns 0 (neutral) when none of
// m's entities are present in aff.
func candidateAffinityScore(m Movie, aff MovieAffinity) float64 {
	var sum float64
	var n int
	add := func(score int) {
		sum += float64(score)
		n++
	}

	for _, id := range m.Genres {
		if s, ok := aff.Genres[id]; ok {
			add(s)
		}
	}
	for _, id := range m.Keywords {
		if s, ok := aff.Keywords[id]; ok {
			add(s)
		}
	}
	for _, id := range m.CastIDs {
		if s, ok := aff.Actors[id]; ok {
			add(s)
		}
	}
	if m.DirectorID != nil {
		if s, ok := aff.Directors[*m.DirectorID]; ok {
			add(s)
		}
	}
	if m.CollectionID != nil {
		if s, ok := aff.Collections[*m.CollectionID]; ok {
			add(s)
		}
	}
	for _, id := range m.ProductionCompanies {
		if s, ok := aff.ProductionCompanies[id]; ok {
			add(s)
		}
	}

	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// rankByAffinity orders candidates by descending candidateAffinityScore —
// the actual computed recommendation-profile match — with ties broken by
// movie id for a stable, deterministic ordering across repeated calls (so
// paging through results, combined with seen-id exclusion, doesn't reshuffle
// already-served titles).
func rankByAffinity(candidates []Movie, aff MovieAffinity) []Movie {
	type scored struct {
		movie Movie
		score float64
	}
	scoredList := make([]scored, len(candidates))
	for i, m := range candidates {
		scoredList[i] = scored{movie: m, score: candidateAffinityScore(m, aff)}
	}
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].score != scoredList[j].score {
			return scoredList[i].score > scoredList[j].score
		}
		return scoredList[i].movie.MovieID < scoredList[j].movie.MovieID
	})

	result := make([]Movie, len(scoredList))
	for i, s := range scoredList {
		result[i] = s.movie
	}
	return result
}
