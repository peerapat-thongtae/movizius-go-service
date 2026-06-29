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
