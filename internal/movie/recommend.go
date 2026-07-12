package movie

import (
	"math"
	"math/rand"
	"sort"
)

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

// weightFromScore maps a -100..100 affinity score to a positive sampling
// weight: 0 (neutral/unknown) maps to 1, positive scores increase it,
// negative scores decrease it (but never to 0, so low-affinity candidates
// can still surface occasionally, just less often).
func weightFromScore(score float64) float64 {
	return math.Pow(2, score/50)
}

// weightedRank orders candidates randomly but biased toward higher affinity
// via weighted reservoir sampling (Efraimidis-Spirakis: key = u^(1/weight)
// for u ~ Uniform(0,1), highest keys first). This keeps /movie/recommendations
// from returning the exact same ordering on every call while still favoring
// titles that match the user's profile more strongly.
func weightedRank(candidates []Movie, aff MovieAffinity) []Movie {
	type keyed struct {
		movie Movie
		key   float64
	}
	keys := make([]keyed, len(candidates))
	for i, m := range candidates {
		w := weightFromScore(candidateAffinityScore(m, aff))
		u := rand.Float64()
		if u <= 0 {
			u = 1e-9
		}
		keys[i] = keyed{movie: m, key: math.Pow(u, 1/w)}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].key > keys[j].key })

	result := make([]Movie, len(keys))
	for i, k := range keys {
		result[i] = k.movie
	}
	return result
}
