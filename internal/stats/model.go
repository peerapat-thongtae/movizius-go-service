// Package stats computes watch-history summaries (totals, ratings, genre/cast/
// director/language breakdowns) for a user, optionally scoped to a month or year.
package stats

import (
	"time"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/tv"
)

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

// RatingBucket is the number of titles given a rounded personal rating.
type RatingBucket struct {
	Rating int `json:"rating"`
	Count  int `json:"count"`
}

// LibrarySnapshot describes the user's current queue, independent of the
// requested time range.
type LibrarySnapshot struct {
	MoviesToWatch int `json:"movies_to_watch"`
	TVToWatch     int `json:"tv_to_watch"`
	TVInProgress  int `json:"tv_in_progress"`
}

// Completion describes titles fully completed in the user's library.
type Completion struct {
	Movies  int `json:"movies"`
	TVShows int `json:"tv_shows"`
}

// ReleaseEra groups watched titles by decade of their release or first-air date.
type ReleaseEra struct {
	Decade string `json:"decade"`
	Count  int    `json:"count"`
}

// IDCount pairs a TMDB entity id (genre, person) with how many watched titles
// referenced it. Person entries also include their display name so the client
// does not need to fan out into one request per person.
type IDCount struct {
	ID    int64  `json:"id"`
	Name  string `json:"name,omitempty"`
	Count int    `json:"count"`
}

// LanguageCount pairs an ISO-639-1 original_language code with how many
// watched titles used it.
type LanguageCount struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

// Summary is the response shape for GET /stats/summary.
type Summary struct {
	Period                      Period          `json:"period"`
	Totals                      Totals          `json:"totals"`
	Ratings                     Ratings         `json:"ratings"`
	RatingDistribution          []RatingBucket  `json:"rating_distribution"`
	CommunityRatingDistribution []RatingBucket  `json:"community_rating_distribution"`
	LibrarySnapshot             LibrarySnapshot `json:"library_snapshot"`
	Completion                  Completion      `json:"completion"`
	FavouriteReleaseEras        []ReleaseEra    `json:"favourite_release_eras"`
	TopGenres                   []IDCount       `json:"top_genres"`
	TopActors                   []IDCount       `json:"top_actors"`
	TopDirectors                []IDCount       `json:"top_directors"` // movie directors + tv creators, merged by TMDB person id
	Languages                   []LanguageCount `json:"languages"`
}

// PersonTitle is a watched title connected to a top actor or director.
type PersonTitle struct {
	ID           int64     `bson:"id" json:"id"`
	MediaType    string    `bson:"media_type" json:"media_type"`
	Title        string    `bson:"title" json:"title"`
	PosterPath   string    `bson:"poster_path" json:"poster_path"`
	ReleaseDate  string    `bson:"release_date" json:"release_date,omitempty"`
	FirstAirDate string    `bson:"first_air_date" json:"first_air_date,omitempty"`
	WatchedAt    time.Time `bson:"watched_at" json:"-"`
}

// PersonTitles is composed of the same full response types used by the normal
// movie and TV list endpoints, including poster and rating data.
type PersonTitles struct {
	Movies []movie.MovieResponse `json:"movies"`
	TV     []tv.TVResponse       `json:"tv"`
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
	ReleaseDate      string   `bson:"release_date"`
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
	FirstAirDate     string   `bson:"first_air_date"`
	EpisodesInPeriod int      `bson:"episodes_in_period"`
}
