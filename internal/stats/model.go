// Package stats computes watch-history summaries (totals, ratings, genre/cast/
// director/language breakdowns) for a user, optionally scoped to a month or year.
package stats

// Period describes the date range and media-type scope a Summary was computed over.
type Period struct {
	Type      string `json:"type"` // "month", "year", or "all"
	Year      int    `json:"year,omitempty"`
	Month     int    `json:"month,omitempty"`
	MediaType string `json:"media_type"` // "movie", "tv", or "all"
}

// Totals holds the raw counts and aggregate watch time for a Summary.
type Totals struct {
	MoviesWatched         int `json:"movies_watched"`
	TVShowsWatched        int `json:"tv_shows_watched"`
	EpisodesWatched       int `json:"episodes_watched"`
	TotalWatchTimeMinutes int `json:"total_watch_time_minutes"`
}

// Ratings holds the rating averages for a Summary.
type Ratings struct {
	MyAverageRating     *float64 `json:"my_average_rating"`
	MyRatedCount        int      `json:"my_rated_count"`
	OthersAverageRating *float64 `json:"others_average_rating"`
}

// IDCount pairs a TMDB entity id (genre, person) with how many watched titles
// referenced it. Names are resolved client-side from the id, matching the
// convention already used by internal/recommendation.Profile.
type IDCount struct {
	ID    int64 `json:"id"`
	Count int   `json:"count"`
}

// LanguageCount pairs an ISO-639-1 original_language code with how many
// watched titles used it.
type LanguageCount struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

// Summary is the response shape for GET /stats/summary.
type Summary struct {
	Period       Period          `json:"period"`
	Totals       Totals          `json:"totals"`
	Ratings      Ratings         `json:"ratings"`
	TopGenres    []IDCount       `json:"top_genres"`
	TopActors    []IDCount       `json:"top_actors"`
	TopDirectors []IDCount       `json:"top_directors"` // movie directors + tv creators, merged by TMDB person id
	Languages    []LanguageCount `json:"languages"`
}

// movieWatchRecord is one watched movie joined with its cached TMDB metadata.
type movieWatchRecord struct {
	Rating           *float64 `bson:"rating"`
	VoteAverage      *float64 `bson:"vote_average"`
	Runtime          *int     `bson:"runtime"`
	Genres           []int64  `bson:"genres"`
	CastIDs          []int64  `bson:"cast_ids"`
	DirectorID       *int64   `bson:"director_id"`
	OriginalLanguage string   `bson:"original_language"`
}

// tvWatchRecord is one TV show with at least one episode watched in the
// requested period, joined with its cached TMDB metadata.
type tvWatchRecord struct {
	Rating           *float64 `bson:"rating"`
	VoteAverage      *float64 `bson:"vote_average"`
	EpisodeRunTime   []int    `bson:"episode_run_time"`
	Genres           []int64  `bson:"genres"`
	CastIDs          []int64  `bson:"cast_ids"`
	CreatorIDs       []int64  `bson:"creator_ids"`
	OriginalLanguage string   `bson:"original_language"`
	EpisodesInPeriod int      `bson:"episodes_in_period"`
}
