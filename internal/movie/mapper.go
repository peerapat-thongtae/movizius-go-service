package movie

import "time"

// movieToModel maps a MovieResponse (TMDB API shape) to a Movie (DB model),
// handling type conversions. Fields owned by another sync (vote_average,
// vote_count) are left at zero — callers put them in $setOnInsert instead.
func movieToModel(data MovieResponse, now time.Time) Movie {
	genres := make([]int64, len(data.Genres))
	for i, g := range data.Genres {
		genres[i] = g.ID
	}

	companies := make([]int64, len(data.ProductionCompanies))
	for i, c := range data.ProductionCompanies {
		companies[i] = c.ID
	}

	var collectionID *int64
	if data.BelongsToCollection != nil {
		id := data.BelongsToCollection.ID
		collectionID = &id
	}

	releaseDateTH := make([]any, len(data.ReleaseDateTH))
	for i, d := range data.ReleaseDateTH {
		releaseDateTH[i] = d
	}

	runtime := data.Runtime
	return Movie{
		MovieID:             data.ID,
		Title:               data.Title,
		OriginalTitle:       data.OriginalTitle,
		PosterPath:          data.PosterPath,
		OriginalLanguage:    data.OriginalLanguage,
		ImdbID:              data.ImdbID,
		Status:              data.Status,
		Popularity:          &data.Popularity,
		Genres:              genres,
		ProductionCompanies: companies,
		ReleaseDateTH:       releaseDateTH,
		CollectionID:        collectionID,
		MediaType:           "movie",
		ReleaseDate:         data.ReleaseDate,
		Runtime:             &runtime,
		WatchProviders:      extractProviderIDs(data.WatchProviders),
		UpdatedAt:           now,
	}
}
