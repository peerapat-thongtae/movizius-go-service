package tv

import "time"

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
