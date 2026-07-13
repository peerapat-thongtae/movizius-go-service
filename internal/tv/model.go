package tv

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TV represents a TV series document in the tv collection (TMDB metadata cache).
type TV struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"              json:"-"`
	TVID                int64              `bson:"id"                         json:"id"`
	Name                string             `bson:"name"                       json:"name"`
	OriginalName        string             `bson:"original_name"              json:"original_name"`
	MediaType           string             `bson:"media_type"                 json:"media_type"`
	PosterPath          string             `bson:"poster_path"                json:"poster_path"`
	OriginalLanguage    string             `bson:"original_language"          json:"original_language"`
	ImdbID              string             `bson:"imdb_id"                    json:"imdb_id"`
	Status              string             `bson:"status"                     json:"status"`
	FirstAirDate        string             `bson:"first_air_date"             json:"first_air_date"`
	LastAirDate         string             `bson:"last_air_date"              json:"last_air_date"`
	IsAnime             bool               `bson:"is_anime"                   json:"is_anime"`
	NumberOfSeasons     *int               `bson:"number_of_seasons"          json:"number_of_seasons"`
	NumberOfEpisodes    *int               `bson:"number_of_episodes"         json:"number_of_episodes"`
	VoteAverage         *float64           `bson:"vote_average"               json:"vote_average"`
	Type                *string            `bson:"type"                       json:"type"`
	VoteCount           *int64             `bson:"vote_count"                 json:"vote_count"`
	Popularity          *float64           `bson:"popularity"                 json:"popularity"`
	Genres              []int64            `bson:"genres"                     json:"genres"`
	ProductionCompanies []int64            `bson:"production_companies"       json:"production_companies"`
	Keywords            []int64            `bson:"keywords"                   json:"keywords"`
	CastIDs             []int64            `bson:"cast_ids"                   json:"cast_ids"`
	CreatorIDs          []int64            `bson:"creator_ids"                json:"creator_ids"`
	Seasons             []any              `bson:"seasons"                    json:"seasons"`
	LastEpisodeToAir    any                `bson:"last_episode_to_air"        json:"last_episode_to_air"`
	NextEpisodeToAir    any                `bson:"next_episode_to_air"        json:"next_episode_to_air"`
	WatchProviders      []int64            `bson:"watch_providers"            json:"watch_providers"`
	UpdatedAt           time.Time          `bson:"updated_at"                 json:"-"`
}

// EpisodeWatched records a single episode the user has watched.
type EpisodeWatched struct {
	EpisodeID     *int64    `bson:"episode_id,omitempty" json:"episode_id,omitempty"`
	SeasonNumber  int       `bson:"season_number"        json:"season_number"`
	EpisodeNumber int       `bson:"episode_number"       json:"episode_number"`
	WatchedAt     time.Time `bson:"watched_at"           json:"watched_at"`
}

// TVState is the aggregated response shape for a user's TV tracking record,
// joining tv_user with the tv collection and computing derived fields.
type TVState struct {
	TVID             int64            `bson:"id"                  json:"id"`
	UserID           string           `bson:"user_id"             json:"user_id"`
	Name             string           `bson:"name"                json:"name"`
	IsAnime          bool             `bson:"is_anime"            json:"is_anime"`
	NumberOfEpisodes *int             `bson:"number_of_episodes"  json:"number_of_episodes"`
	NumberOfSeasons  *int             `bson:"number_of_seasons"   json:"number_of_seasons"`
	EpisodeWatched   []EpisodeWatched `bson:"episode_watched"     json:"episode_watched"`
	LatestWatched    *time.Time       `bson:"latest_watched"      json:"latest_watched,omitempty"`
	WatchlistedAt    time.Time        `bson:"watchlisted_at"      json:"watchlisted_at"`
	CountWatched     int              `bson:"count_watched"       json:"count_watched"`
	AccountStatus    string           `bson:"account_status"      json:"account_status"`
	LatestState      *time.Time       `bson:"latest_state"        json:"latest_state,omitempty"`
	MaxWatchedEp     *EpisodeWatched  `bson:"max_watched_ep"      json:"max_watched_ep,omitempty"`
}

// NextEpisodeAirDateUpdate carries the fields needed to patch next_episode_to_air.air_date.
type NextEpisodeAirDateUpdate struct {
	ImdbID        string
	SeasonNumber  int
	EpisodeNumber int
	AirDate       string // RFC3339 timestamp from TVMaze airstamp
}

// TVUser represents a user's tracking record for a TV series in the tv_user collection.
type TVUser struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"            json:"-"`
	TVID           int64              `bson:"id"                       json:"id"`
	UserID         string             `bson:"user_id"                  json:"user_id"`
	MediaType      string             `bson:"media_type"               json:"media_type"`
	EpisodeWatched []EpisodeWatched   `bson:"episode_watched"          json:"episode_watched"`
	WatchlistedAt  time.Time          `bson:"watchlisted_at"           json:"watchlisted_at"`
	WatchedAt      *time.Time         `bson:"watched_at,omitempty"     json:"watched_at,omitempty"`
	UpdatedAt      time.Time          `bson:"updated_at"               json:"-"`
	AccountStatus  string             `bson:"account_status,omitempty" json:"account_status,omitempty"`
	Rating         *float64           `bson:"rating,omitempty"         json:"rating,omitempty"`

	// ProfileContribution snapshots the recommendation-profile contribution
	// last derived from this doc's state, so a later state change (rating
	// edit, episode changes) can be applied to the profile as a delta instead
	// of a blind re-add, avoiding double counting.
	ProfileContribution *ProfileContribution `bson:"profile_contribution,omitempty" json:"-"`
}

// ProfileContribution is the recommendation profile bookkeeping snapshot
// stored on a TVUser doc (see recommendationProfile feature).
type ProfileContribution struct {
	Contribution float64 `bson:"contribution"`
	Weight       float64 `bson:"weight"`
	Applied      bool    `bson:"applied"`
	Version      int     `bson:"version"`
}
