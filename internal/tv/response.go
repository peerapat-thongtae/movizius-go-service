package tv

import (
	"encoding/json"
	"time"
)

// TV represents a TV series document in the tv collection (TMDB metadata cache).
// JSON keys mirror the TMDB API / Dart model (snake_case); BSON keys mirror MongoDB storage.
type TVResponse struct {
	Adult               bool                `bson:"adult"                  json:"adult"`
	BackdropPath        string              `bson:"backdrop_path"          json:"backdrop_path"`
	CreatedBy           []CreatedBy         `bson:"created_by"             json:"created_by"`
	EpisodeRunTime      []int               `bson:"episode_run_time"       json:"episode_run_time"`
	FirstAirDate        string              `bson:"first_air_date"         json:"first_air_date"`
	Genres              []Genre             `bson:"genres"                 json:"genres"`
	Homepage            string              `bson:"homepage"               json:"homepage"`
	ID                  int64               `bson:"id"                     json:"id"`
	InProduction        bool                `bson:"in_production"          json:"in_production"`
	Languages           []string            `bson:"languages"              json:"languages"`
	LastAirDate         string              `bson:"last_air_date"          json:"last_air_date"`
	LastEpisodeToAir    *Episode            `bson:"last_episode_to_air"    json:"last_episode_to_air"`
	Name                string              `bson:"name"                   json:"name"`
	NextEpisodeToAir    *Episode            `bson:"next_episode_to_air"    json:"next_episode_to_air"`
	Networks            []Network           `bson:"networks"               json:"networks"`
	NumberOfEpisodes    int                 `bson:"number_of_episodes"     json:"number_of_episodes"`
	NumberOfSeasons     int                 `bson:"number_of_seasons"      json:"number_of_seasons"`
	OriginCountry       []string            `bson:"origin_country"         json:"origin_country"`
	OriginalLanguage    string              `bson:"original_language"      json:"original_language"`
	OriginalName        string              `bson:"original_name"          json:"original_name"`
	Overview            string              `bson:"overview"               json:"overview"`
	Popularity          float64             `bson:"popularity"             json:"popularity"`
	PosterPath          string              `bson:"poster_path"            json:"poster_path"`
	ProductionCompanies []ProductionCompany `bson:"production_companies"   json:"production_companies"`
	ProductionCountries []ProductionCountry `bson:"production_countries"   json:"production_countries"`
	Seasons             []Season            `bson:"seasons"                json:"seasons"`
	Softcore            bool                `bson:"softcore"               json:"softcore"`
	SpokenLanguages     []SpokenLanguage    `bson:"spoken_languages"       json:"spoken_languages"`
	Status              string              `bson:"status"                 json:"status"`
	Tagline             string              `bson:"tagline"                json:"tagline"`
	Type                string              `bson:"type"                   json:"type"`
	VoteAverage         float64             `bson:"vote_average"           json:"vote_average"`
	VoteCount           int64               `bson:"vote_count"             json:"vote_count"`
	Credits             *Credits            `bson:"credits"                json:"credits"`
	ExternalIDs         *ExternalIDs        `bson:"external_ids"           json:"external_ids"`
	Videos              *Videos             `bson:"videos"                 json:"videos"`
	MediaType           string              `bson:"media_type"             json:"media_type,omitempty"`
	ImdbID              string              `bson:"imdb_id"                json:"imdb_id,omitempty"`
	IsAnime             bool                `bson:"is_anime"               json:"is_anime"`
	WatchProviders      *WatchProviders     `bson:"watch_providers"        json:"watch_providers"`
	UpdatedAt           time.Time           `bson:"updated_at"             json:"-"`
}

// StateEpisode is a trimmed episode shape used in TVStateResponse (last/next episode to air).
type StateEpisode struct {
	ID            int64  `bson:"id"             json:"id"`
	AirDate       string `bson:"air_date"       json:"air_date"`
	EpisodeNumber int    `bson:"episode_number" json:"episode_number"`
	EpisodeType   string `bson:"episode_type"   json:"episode_type"`
	SeasonNumber  int    `bson:"season_number"  json:"season_number"`
}

// StateSeason is a trimmed season shape used in TVStateResponse.
type StateSeason struct {
	ID           int64   `bson:"id"            json:"id"`
	AirDate      *string `bson:"air_date"      json:"air_date"`
	EpisodeCount int     `bson:"episode_count" json:"episode_count"`
	Name         string  `bson:"name"          json:"name"`
	SeasonNumber int     `bson:"season_number" json:"season_number"`
}

// TVStateResponse is the API response shape for GET /tv/states, matching the client TVStates interface.
type TVStateResponse struct {
	TVID             int64            `bson:"id"                  json:"id"`
	UserID           string           `bson:"user_id"             json:"user_id"`
	Name             string           `bson:"name"                json:"name"`
	MediaType        string           `bson:"media_type"          json:"media_type"`
	IsAnime          bool             `bson:"is_anime"            json:"is_anime"`
	VoteAverage      float64          `bson:"vote_average"        json:"vote_average"`
	VoteCount        int64            `bson:"vote_count"          json:"vote_count"`
	NumberOfEpisodes *int             `bson:"number_of_episodes"  json:"number_of_episodes"`
	NumberOfSeasons  *int             `bson:"number_of_seasons"   json:"number_of_seasons"`
	EpisodeWatched   []EpisodeWatched `bson:"episode_watched"     json:"episode_watched"`
	LatestWatched    *time.Time       `bson:"latest_watched"      json:"latest_watched,omitempty"`
	WatchlistedAt    time.Time        `bson:"watchlisted_at"      json:"watchlisted_at"`
	CountWatched     int              `bson:"count_watched"       json:"count_watched"`
	AccountStatus    string           `bson:"account_status"      json:"account_status"`
	LatestState      *time.Time       `bson:"latest_state"        json:"latest_state,omitempty"`
	MaxWatchedEp     *EpisodeWatched  `bson:"max_watched_ep"      json:"max_watched_ep,omitempty"`
	NextEpisodeToAir *StateEpisode    `bson:"next_episode_to_air" json:"next_episode_to_air"`
	LastEpisodeToAir *StateEpisode    `bson:"last_episode_to_air" json:"last_episode_to_air"`
	Seasons          []StateSeason    `bson:"seasons"             json:"seasons"`
	WatchedSeasons   []int            `bson:"watched_seasons"     json:"watched_seasons"`
}

// UnmarshalJSON remaps TMDB's "watch/providers" key to the struct's WatchProviders field.
func (r *TVResponse) UnmarshalJSON(data []byte) error {
	type Alias TVResponse
	var raw struct {
		Alias
		WatchProvidersIn *WatchProviders `json:"watch/providers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = TVResponse(raw.Alias)
	if raw.WatchProvidersIn != nil {
		r.WatchProviders = raw.WatchProvidersIn
	}
	return nil
}

type CreatedBy struct {
	ID           int64   `bson:"id"            json:"id"`
	CreditID     string  `bson:"credit_id"     json:"credit_id"`
	Name         string  `bson:"name"          json:"name"`
	OriginalName string  `bson:"original_name" json:"original_name"`
	Gender       int     `bson:"gender"        json:"gender"`
	ProfilePath  *string `bson:"profile_path"  json:"profile_path"`
}

type Genre struct {
	ID   int64  `bson:"id"   json:"id"`
	Name string `bson:"name" json:"name"`
}

// Episode merges LastEpisodeToAir and NextEpisodeToAir — the only difference
// is that runtime/still_path are optional on the latter, which nullable fields cover.
type Episode struct {
	ID             int64   `bson:"id"              json:"id"`
	Name           string  `bson:"name"            json:"name"`
	Overview       string  `bson:"overview"        json:"overview"`
	VoteAverage    float64 `bson:"vote_average"    json:"vote_average"`
	VoteCount      int     `bson:"vote_count"      json:"vote_count"`
	AirDate        string  `bson:"air_date"        json:"air_date"`
	EpisodeNumber  int     `bson:"episode_number"  json:"episode_number"`
	EpisodeType    string  `bson:"episode_type"    json:"episode_type"`
	ProductionCode string  `bson:"production_code" json:"production_code"`
	Runtime        *int    `bson:"runtime"         json:"runtime"`
	SeasonNumber   int     `bson:"season_number"   json:"season_number"`
	ShowID         int64   `bson:"show_id"         json:"show_id"`
	StillPath      *string `bson:"still_path"      json:"still_path"`
}

type Network struct {
	ID            int64  `bson:"id"             json:"id"`
	LogoPath      string `bson:"logo_path"      json:"logo_path"`
	Name          string `bson:"name"           json:"name"`
	OriginCountry string `bson:"origin_country" json:"origin_country"`
}

type ProductionCompany struct {
	ID            int64   `bson:"id"             json:"id"`
	LogoPath      *string `bson:"logo_path"      json:"logo_path"`
	Name          string  `bson:"name"           json:"name"`
	OriginCountry string  `bson:"origin_country" json:"origin_country"`
}

type ProductionCountry struct {
	ISO31661 string `bson:"iso_3166_1" json:"iso_3166_1"`
	Name     string `bson:"name"       json:"name"`
}

type Season struct {
	AirDate      *string `bson:"air_date"       json:"air_date"`
	EpisodeCount int     `bson:"episode_count"  json:"episode_count"`
	ID           int64   `bson:"id"             json:"id"`
	Name         string  `bson:"name"           json:"name"`
	Overview     string  `bson:"overview"       json:"overview"`
	PosterPath   *string `bson:"poster_path"    json:"poster_path"`
	SeasonNumber int     `bson:"season_number"  json:"season_number"`
	VoteAverage  float64 `bson:"vote_average"   json:"vote_average"`
}

type SpokenLanguage struct {
	EnglishName string `bson:"english_name" json:"english_name"`
	ISO6391     string `bson:"iso_639_1"    json:"iso_639_1"`
	Name        string `bson:"name"         json:"name"`
}

type Credits struct {
	Cast []CastMember `bson:"cast" json:"cast"`
	Crew []CastMember `bson:"crew" json:"crew"`
}

// CastMember merges cast and crew into one shape; character/order populate for
// cast members, department/job for crew members.
type CastMember struct {
	Adult              bool    `bson:"adult"                json:"adult"`
	Gender             int     `bson:"gender"               json:"gender"`
	ID                 int64   `bson:"id"                   json:"id"`
	KnownForDepartment string  `bson:"known_for_department" json:"known_for_department"`
	Name               string  `bson:"name"                 json:"name"`
	OriginalName       string  `bson:"original_name"        json:"original_name"`
	Popularity         float64 `bson:"popularity"           json:"popularity"`
	ProfilePath        *string `bson:"profile_path"         json:"profile_path"`
	Character          *string `bson:"character,omitempty"  json:"character,omitempty"`
	CreditID           string  `bson:"credit_id"            json:"credit_id"`
	Order              *int    `bson:"order,omitempty"      json:"order,omitempty"`
	Department         *string `bson:"department,omitempty" json:"department,omitempty"`
	Job                *string `bson:"job,omitempty"        json:"job,omitempty"`
}

type ExternalIDs struct {
	ImdbID      string  `bson:"imdb_id"       json:"imdb_id"`
	FreebaseMid *string `bson:"freebase_mid"  json:"freebase_mid"`
	FreebaseID  *string `bson:"freebase_id"   json:"freebase_id"`
	TVDBId      *int64  `bson:"tvdb_id"       json:"tvdb_id"`
	TVRageID    *int64  `bson:"tvrage_id"     json:"tvrage_id"`
	WikidataID  *string `bson:"wikidata_id"   json:"wikidata_id"`
	FacebookID  *string `bson:"facebook_id"   json:"facebook_id"`
	InstagramID *string `bson:"instagram_id"  json:"instagram_id"`
	TwitterID   *string `bson:"twitter_id"    json:"twitter_id"`
}

type Videos struct {
	Results []VideoResult `bson:"results" json:"results"`
}

type VideoResult struct {
	ISO6391     string     `bson:"iso_639_1"    json:"iso_639_1"`
	ISO31661    string     `bson:"iso_3166_1"   json:"iso_3166_1"`
	Name        string     `bson:"name"         json:"name"`
	Key         string     `bson:"key"          json:"key"`
	Site        string     `bson:"site"         json:"site"`
	Size        int        `bson:"size"         json:"size"`
	Type        string     `bson:"type"         json:"type"`
	Official    bool       `bson:"official"     json:"official"`
	ID          string     `bson:"id"           json:"id"`
	PublishedAt *time.Time `bson:"published_at" json:"published_at"`
}

type WatchProviders struct {
	Results map[string]*WatchProviderCountry `bson:"results" json:"results"`
}

type WatchProviderCountry struct {
	Link     string     `bson:"link"               json:"link"`
	Flatrate []Flatrate `bson:"flatrate,omitempty" json:"flatrate,omitempty"`
	Rent     []Flatrate `bson:"rent,omitempty"     json:"rent,omitempty"`
	Buy      []Flatrate `bson:"buy,omitempty"      json:"buy,omitempty"`
	Ads      []Flatrate `bson:"ads,omitempty"      json:"ads,omitempty"`
	Free     []Flatrate `bson:"free,omitempty"     json:"free,omitempty"`
}

type Flatrate struct {
	LogoPath        string `bson:"logo_path"        json:"logo_path"`
	ProviderID      int64  `bson:"provider_id"      json:"provider_id"`
	ProviderName    string `bson:"provider_name"    json:"provider_name"`
	DisplayPriority int    `bson:"display_priority" json:"display_priority"`
}
