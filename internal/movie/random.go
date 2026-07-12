package movie

import (
	"math"
	"math/rand"
	"sort"
)

// affinityScore averages the user's recommendation-profile scores across
// every entity of m that appears in aff (genres, keywords, top-billed cast,
// director, collection, production companies). Returns 0 (neutral) when
// none of m's entities are present in aff, including when aff is the
// zero-value MovieAffinity — that keeps weightedSample's behavior identical
// to a uniform shuffle when no recommendation profile is available.
func affinityScore(m MovieResponse, aff MovieAffinity) float64 {
	var sum float64
	var n int
	add := func(score int) {
		sum += float64(score)
		n++
	}

	for _, g := range m.Genres {
		if s, ok := aff.Genres[g.ID]; ok {
			add(s)
		}
	}
	if m.Keywords != nil {
		for _, k := range m.Keywords.Keywords {
			if s, ok := aff.Keywords[k.ID]; ok {
				add(s)
			}
		}
	}
	if m.Casts != nil {
		for _, c := range topCastIDs(m.Casts, topCastN) {
			if s, ok := aff.Actors[c]; ok {
				add(s)
			}
		}
		if directorID := findDirectorID(m.Casts); directorID != nil {
			if s, ok := aff.Directors[*directorID]; ok {
				add(s)
			}
		}
	}
	if m.BelongsToCollection != nil {
		if s, ok := aff.Collections[m.BelongsToCollection.ID]; ok {
			add(s)
		}
	}
	for _, c := range m.ProductionCompanies {
		if s, ok := aff.ProductionCompanies[c.ID]; ok {
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
// negative scores decrease it (but never to 0, so disliked titles can still
// appear, just less often).
func weightFromScore(score float64) float64 {
	return math.Pow(2, score/50)
}

// weightedSample returns up to `total` items from pool, order randomized but
// biased toward higher-affinity candidates via weighted reservoir sampling
// (Efraimidis-Spirakis: key = u^(1/weight) for u ~ Uniform(0,1), take the
// items with the largest keys). When every weight is 1 (empty/neutral
// affinity), this reduces to an unbiased random shuffle, so it's a strict
// generalization of the previous plain-shuffle behavior.
func weightedSample(pool []MovieResponse, aff MovieAffinity, total int) []MovieResponse {
	type keyed struct {
		movie MovieResponse
		key   float64
	}
	keys := make([]keyed, len(pool))
	for i, m := range pool {
		w := weightFromScore(affinityScore(m, aff))
		u := rand.Float64()
		if u <= 0 {
			u = 1e-9
		}
		keys[i] = keyed{movie: m, key: math.Pow(u, 1/w)}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].key > keys[j].key })

	if len(keys) > total {
		keys = keys[:total]
	}
	result := make([]MovieResponse, len(keys))
	for i, k := range keys {
		result[i] = k.movie
	}
	return result
}
