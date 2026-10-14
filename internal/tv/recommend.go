package tv

import "sort"

func candidateAffinityScore(t TV, aff TVAffinity) float64 {
	var sum float64
	var count int
	add := func(score int) { sum += float64(score); count++ }
	for _, id := range t.Genres {
		if score, ok := aff.Genres[id]; ok {
			add(score)
		}
	}
	for _, id := range t.Keywords {
		if score, ok := aff.Keywords[id]; ok {
			add(score)
		}
	}
	for _, id := range t.CastIDs {
		if score, ok := aff.Actors[id]; ok {
			add(score)
		}
	}
	for _, id := range t.CreatorIDs {
		if score, ok := aff.Creators[id]; ok {
			add(score)
		}
	}
	for _, id := range t.ProductionCompanies {
		if score, ok := aff.ProductionCompanies[id]; ok {
			add(score)
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func rankByAffinity(candidates []TV, aff TVAffinity) []TV {
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidateAffinityScore(candidates[i], aff), candidateAffinityScore(candidates[j], aff)
		if left != right {
			return left > right
		}
		return candidates[i].TVID < candidates[j].TVID
	})
	return candidates
}
