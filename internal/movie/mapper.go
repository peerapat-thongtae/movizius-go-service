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

	releaseDateTH := extractReleaseDateTH(data)

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

// overlayDBFields lets the cached DB record win over the freshly-fetched TMDB
// detail for the fields the DB is the source of truth for (popularity from the
// daily export sync, vote_average/vote_count from IMDB, and the stored catalog
// fields). Only non-empty DB values are applied. Lossy DB fields stored as ID
// arrays (genres, production_companies, watch_providers) are intentionally left
// as TMDB's richer objects.
func overlayDBFields(detail *MovieResponse, db Movie) {
	if db.VoteAverage != nil {
		detail.VoteAverage = *db.VoteAverage
	}
	if db.VoteCount != nil {
		detail.VoteCount = *db.VoteCount
	}
	if db.Popularity != nil {
		detail.Popularity = *db.Popularity
	}
	if db.Runtime != nil {
		detail.Runtime = *db.Runtime
	}
	if db.ReleaseDate != "" {
		detail.ReleaseDate = db.ReleaseDate
	}
	if db.ReleaseDateTH != "" {
		detail.ReleaseDateTH = db.ReleaseDateTH
	}
	if db.Status != "" {
		detail.Status = db.Status
	}
	if db.Title != "" {
		detail.Title = db.Title
	}
	if db.OriginalTitle != "" {
		detail.OriginalTitle = db.OriginalTitle
	}
	if db.PosterPath != "" {
		detail.PosterPath = db.PosterPath
	}
	if db.OriginalLanguage != "" {
		detail.OriginalLanguage = db.OriginalLanguage
	}
}
