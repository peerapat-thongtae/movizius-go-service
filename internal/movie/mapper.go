package movie

import (
	"sort"
	"time"
)

// topCastMultiplier is the number of top-billed cast members kept for the
// recommendation profile's actor bucket.
const topCastN = 5

// topCastIDs returns up to n cast member ids ordered by billing (Order
// ascending); cast members with no Order are placed last.
func topCastIDs(c *Casts, n int) []int64 {
	if c == nil || len(c.Cast) == 0 {
		return nil
	}
	sorted := make([]Cast, len(c.Cast))
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

// findDirectorID returns the id of the first crew member whose job is
// "Director", or nil if none is found.
func findDirectorID(c *Casts) *int64 {
	if c == nil {
		return nil
	}
	for _, m := range c.Crew {
		if m.Job != nil && *m.Job == "Director" {
			id := m.ID
			return &id
		}
	}
	return nil
}

// keywordIDs flattens a movie's keywords wrapper into a slice of TMDB keyword ids.
func keywordIDs(k *KeywordsWrapper) []int64 {
	if k == nil {
		return nil
	}
	ids := make([]int64, len(k.Keywords))
	for i, kw := range k.Keywords {
		ids[i] = kw.ID
	}
	return ids
}

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
		Keywords:            keywordIDs(data.Keywords),
		CastIDs:             topCastIDs(data.Casts, topCastN),
		DirectorID:          findDirectorID(data.Casts),
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
