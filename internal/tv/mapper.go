package tv

import (
	"sort"
	"time"
)

// topCastN is the number of top-billed cast members kept for the
// recommendation profile's actor bucket.
const topCastN = 5

// topCastIDs returns up to n cast member ids ordered by billing (Order
// ascending); cast members with no Order are placed last.
func topCastIDs(c *Credits, n int) []int64 {
	if c == nil || len(c.Cast) == 0 {
		return nil
	}
	sorted := make([]CastMember, len(c.Cast))
	copy(sorted, c.Cast)
	sort.SliceStable(sorted, func(i, j int) bool {
		oi, oj := sorted[i].Order, sorted[j].Order
		if oi == nil {
			return false
		}
		if oj == nil {
			return true
		}
		return *oi < *oj
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	ids := make([]int64, len(sorted))
	for i, m := range sorted {
		ids[i] = m.ID
	}
	return ids
}

// creatorIDs flattens a TV series' created_by list into TMDB person ids.
func creatorIDs(cb []CreatedBy) []int64 {
	if len(cb) == 0 {
		return nil
	}
	ids := make([]int64, len(cb))
	for i, c := range cb {
		ids[i] = c.ID
	}
	return ids
}

// tvKeywordIDs flattens a TV series' keywords wrapper into TMDB keyword ids.
func tvKeywordIDs(k *TVKeywordsWrapper) []int64 {
	if k == nil {
		return nil
	}
	ids := make([]int64, len(k.Results))
	for i, kw := range k.Results {
		ids[i] = kw.ID
	}
	return ids
}

// tvToModel maps a TVResponse (TMDB API shape) to a TV (DB model),
// handling type conversions. Fields owned by another sync (vote_average,
// vote_count) are left at zero — callers put them in $setOnInsert instead.
func tvToModel(data TVResponse, now time.Time) TV {
	genres := make([]int64, len(data.Genres))
	for i, g := range data.Genres {
		genres[i] = g.ID
	}

	companies := make([]int64, len(data.ProductionCompanies))
	for i, c := range data.ProductionCompanies {
		companies[i] = c.ID
	}

	seasons := make([]any, len(data.Seasons))
	for i, s := range data.Seasons {
		seasons[i] = s
	}

	numSeasons := data.NumberOfSeasons
	numEpisodes := data.NumberOfEpisodes
	tvType := data.Type

	return TV{
		TVID:                data.ID,
		Name:                data.Name,
		OriginalName:        data.OriginalName,
		MediaType:           "tv",
		PosterPath:          data.PosterPath,
		OriginalLanguage:    data.OriginalLanguage,
		ImdbID:              data.ImdbID,
		Status:              data.Status,
		FirstAirDate:        data.FirstAirDate,
		LastAirDate:         data.LastAirDate,
		IsAnime:             data.IsAnime,
		NumberOfSeasons:     &numSeasons,
		NumberOfEpisodes:    &numEpisodes,
		Type:                &tvType,
		Popularity:          &data.Popularity,
		Genres:              genres,
		ProductionCompanies: companies,
		Keywords:            tvKeywordIDs(data.Keywords),
		CastIDs:             topCastIDs(data.Credits, topCastN),
		CreatorIDs:          creatorIDs(data.CreatedBy),
		Seasons:             seasons,
		LastEpisodeToAir:    data.LastEpisodeToAir,
		NextEpisodeToAir:    data.NextEpisodeToAir,
		WatchProviders:      extractProviderIDs(data.WatchProviders),
		UpdatedAt:           now,
	}
}

// overlayDBFields lets the cached DB record win over the freshly-fetched TMDB
// detail for the fields the DB is the source of truth for (popularity from the
// daily export sync, vote_average/vote_count from IMDB, the is_anime flag, and
// the stored catalog fields). Only non-empty DB values are applied. Lossy DB
// fields stored as ID arrays (genres, production_companies, watch_providers) and
// next_episode_to_air (handled separately with the DB air date) are left to their
// existing sources. is_anime is always applied — the DB flag is authoritative.
func overlayDBFields(detail *TVResponse, db TV) {
	if db.VoteAverage != nil {
		detail.VoteAverage = *db.VoteAverage
	}
	if db.VoteCount != nil {
		detail.VoteCount = *db.VoteCount
	}
	if db.Popularity != nil {
		detail.Popularity = *db.Popularity
	}
	if db.NumberOfSeasons != nil {
		detail.NumberOfSeasons = *db.NumberOfSeasons
	}
	if db.NumberOfEpisodes != nil {
		detail.NumberOfEpisodes = *db.NumberOfEpisodes
	}
	if db.Type != nil && *db.Type != "" {
		detail.Type = *db.Type
	}
	if db.Status != "" {
		detail.Status = db.Status
	}
	if db.FirstAirDate != "" {
		detail.FirstAirDate = db.FirstAirDate
	}
	if db.LastAirDate != "" {
		detail.LastAirDate = db.LastAirDate
	}
	if db.Name != "" {
		detail.Name = db.Name
	}
	if db.OriginalName != "" {
		detail.OriginalName = db.OriginalName
	}
	if db.PosterPath != "" {
		detail.PosterPath = db.PosterPath
	}
	if db.OriginalLanguage != "" {
		detail.OriginalLanguage = db.OriginalLanguage
	}
	detail.IsAnime = db.IsAnime
}
