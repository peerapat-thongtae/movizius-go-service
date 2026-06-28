package movie

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Movie represents a movie document in the movies collection (TMDB metadata cache).
type Movie struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"                json:"-"`
	MovieID             int64              `bson:"id"                           json:"id"`
	Title               string             `bson:"title"                        json:"title"`
	OriginalTitle       string             `bson:"original_title"               json:"original_title"`
	PosterPath          string             `bson:"poster_path"                  json:"poster_path"`
	OriginalLanguage    string             `bson:"original_language"            json:"original_language"`
	ImdbID              string             `bson:"imdb_id"                      json:"imdb_id"`
	Status              string             `bson:"status"                       json:"status"`
	Popularity          *float64           `bson:"popularity"                   json:"popularity"`
	Genres              []int64            `bson:"genres"                       json:"genres"`
	ProductionCompanies []int64            `bson:"production_companies"         json:"production_companies"`
	ReleaseDateTH       []any              `bson:"release_date_th"              json:"release_date_th"`
	CollectionID        *int64             `bson:"collection_id"                json:"collection_id"`
	MediaType           string             `bson:"media_type"                   json:"media_type"`
	ReleaseDate         string             `bson:"release_date"                 json:"release_date"`
	Runtime             *int               `bson:"runtime,omitempty"            json:"runtime,omitempty"`
	VoteAverage         *float64           `bson:"vote_average"                 json:"vote_average"`
	VoteCount           *int64             `bson:"vote_count"                   json:"vote_count"`
	WatchProviders      []int64            `bson:"watch_providers"              json:"watch_providers"`
	UpdatedAt           time.Time          `bson:"updated_at"                   json:"-"`
}

// MovieUser represents a user's tracking record for a movie in the movie_user collection.
type MovieUser struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"            json:"-"`
	MovieID       int64              `bson:"id"                       json:"id"`
	UserID        string             `bson:"user_id"                  json:"user_id"`
	MediaType     string             `bson:"media_type"               json:"media_type"`
	WatchlistedAt time.Time          `bson:"watchlisted_at"           json:"watchlisted_at"`
	WatchedAt     *time.Time         `bson:"watched_at,omitempty"     json:"watched_at,omitempty"`
	UpdatedAt     time.Time          `bson:"updated_at"               json:"-"`
	AccountStatus *string            `bson:"account_status,omitempty" json:"account_status,omitempty"`
}
