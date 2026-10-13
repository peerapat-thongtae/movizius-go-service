package movie

import (
	"encoding/json"
	"time"
)

// Movie represents a movie document in the movies collection (TMDB metadata cache).
// JSON keys mirror the TMDB API / Dart model (snake_case); BSON keys mirror MongoDB storage.
type MovieResponse struct {
	Adult               bool                  `bson:"adult"                  json:"adult"`
	BackdropPath        string                `bson:"backdrop_path"          json:"backdrop_path"`
	BelongsToCollection *Collection           `bson:"belongs_to_collection"  json:"belongs_to_collection"`
	Budget              int64                 `bson:"budget"                 json:"budget"`
	Genres              []Genre               `bson:"genres"                 json:"genres"`
	Homepage            string                `bson:"homepage"               json:"homepage"`
	ID                  int64                 `bson:"id"                     json:"id"`
	ImdbID              string                `bson:"imdb_id"                json:"imdb_id,omitempty"`
	OriginCountry       []string              `bson:"origin_country"         json:"origin_country"`
	OriginalLanguage    string                `bson:"original_language"      json:"original_language"`
	OriginalTitle       string                `bson:"original_title"         json:"original_title"`
	Overview            string                `bson:"overview"               json:"overview"`
	Popularity          float64               `bson:"popularity"             json:"popularity"`
	PosterPath          string                `bson:"poster_path"            json:"poster_path"`
	ProductionCompanies []ProductionCompany   `bson:"production_companies"   json:"production_companies"`
	ProductionCountries []ProductionCountry   `bson:"production_countries"   json:"production_countries"`
	ReleaseDate         string                `bson:"release_date"           json:"release_date"`
	Revenue             int64                 `bson:"revenue"                json:"revenue"`
	Runtime             int                   `bson:"runtime"                json:"runtime"`
	Softcore            bool                  `bson:"softcore"               json:"softcore"`
	SpokenLanguages     []SpokenLanguage      `bson:"spoken_languages"       json:"spoken_languages"`
	Status              string                `bson:"status"                 json:"status"`
	Tagline             string                `bson:"tagline"                json:"tagline"`
	Title               string                `bson:"title"                  json:"title"`
	Video               bool                  `bson:"video"                  json:"video"`
	VoteAverage         float64               `bson:"vote_average"           json:"vote_average"`
	VoteCount           int64                 `bson:"vote_count"             json:"vote_count"`
	ExternalIDs         *ExternalIDs          `bson:"external_ids"           json:"external_ids"`
	Casts               *Casts                `bson:"casts"                  json:"casts"`
	Videos              *Videos               `bson:"videos"                 json:"videos"`
	ReleaseDates        *ReleaseDates         `bson:"release_dates"          json:"release_dates"`
	MediaType           string                `bson:"media_type"             json:"media_type,omitempty"`
	WatchProviders      *WatchProviderCountry `bson:"watch_providers"        json:"watch_providers"`
	ReleaseDateTH       string                `bson:"release_date_th"        json:"release_date_th"`
	Keywords            *KeywordsWrapper      `bson:"keywords"               json:"keywords"`
	Recommendations     *Recommendations      `bson:"recommendations"        json:"recommendations"`
	UpdatedAt           time.Time             `bson:"updated_at"             json:"-"`
}

// UnmarshalJSON remaps TMDB's non-Go-compatible / differently named append
// response keys while preserving the API shape served to frontend clients.
func (r *MovieResponse) UnmarshalJSON(data []byte) error {
	type Alias MovieResponse
	var raw struct {
		Alias
		WatchProvidersIn *WatchProviders `json:"watch/providers"`
		Credits          *Casts          `json:"credits"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = MovieResponse(raw.Alias)
	if raw.WatchProvidersIn != nil {
		r.WatchProviders = raw.WatchProvidersIn.Results["TH"]
	}
	if raw.Credits != nil {
		r.Casts = raw.Credits
	}
	return nil
}

// extractReleaseDateTH finds the theatrical (type=3) release date for Thailand.
func extractReleaseDateTH(data MovieResponse) string {
	if data.ReleaseDates == nil {
		return ""
	}
	for _, r := range data.ReleaseDates.Results {
		if r.ISO31661 != "TH" {
			continue
		}
		for _, d := range r.ReleaseDates {
			if d.Type == 3 && d.ReleaseDate != "" {
				return d.ReleaseDate
			}
		}
		break
	}
	return ""
}

type Collection struct {
	ID           int64  `bson:"id"            json:"id"`
	Name         string `bson:"name"          json:"name"`
	PosterPath   string `bson:"poster_path"   json:"poster_path"`
	BackdropPath string `bson:"backdrop_path" json:"backdrop_path"`
}

type Genre struct {
	ID   int64  `bson:"id"   json:"id"`
	Name string `bson:"name" json:"name"`
}

// KeywordsWrapper mirrors TMDB movie detail's "keywords" append_to_response shape:
// {"keywords":[{"id":..,"name":..}]}.
type KeywordsWrapper struct {
	Keywords []Keyword `bson:"keywords" json:"keywords"`
}

// Recommendations mirrors TMDB's appended recommendations response.
type Recommendations struct {
	Page         int              `bson:"page"          json:"page"`
	TotalPages   int              `bson:"total_pages"   json:"total_pages"`
	TotalResults int              `bson:"total_results" json:"total_results"`
	Results      []Recommendation `bson:"results"       json:"results"`
}

// Recommendation is a compact movie or TV result returned by TMDB.
type Recommendation struct {
	ID           int64   `bson:"id"             json:"id"`
	Title        string  `bson:"title"          json:"title,omitempty"`
	Name         string  `bson:"name"           json:"name,omitempty"`
	PosterPath   *string `bson:"poster_path"    json:"poster_path"`
	ReleaseDate  string  `bson:"release_date"   json:"release_date,omitempty"`
	FirstAirDate string  `bson:"first_air_date" json:"first_air_date,omitempty"`
	VoteAverage  float64 `bson:"vote_average"   json:"vote_average"`
	VoteCount    int64   `bson:"vote_count"     json:"vote_count"`
}

type Keyword struct {
	ID   int64  `bson:"id"   json:"id"`
	Name string `bson:"name" json:"name"`
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

type SpokenLanguage struct {
	EnglishName string `bson:"english_name" json:"english_name"`
	ISO6391     string `bson:"iso_639_1"    json:"iso_639_1"`
	Name        string `bson:"name"         json:"name"`
}

type Casts struct {
	Cast []Cast `bson:"cast" json:"cast"`
	Crew []Cast `bson:"crew" json:"crew"`
}

type Cast struct {
	Adult              bool    `bson:"adult"                json:"adult"`
	Gender             int     `bson:"gender"               json:"gender"`
	ID                 int64   `bson:"id"                   json:"id"`
	KnownForDepartment string  `bson:"known_for_department" json:"known_for_department"`
	Name               string  `bson:"name"                 json:"name"`
	OriginalName       string  `bson:"original_name"        json:"original_name"`
	Popularity         float64 `bson:"popularity"           json:"popularity"`
	ProfilePath        *string `bson:"profile_path"         json:"profile_path"`
	CastID             *int64  `bson:"cast_id,omitempty"    json:"cast_id,omitempty"`
	Character          *string `bson:"character,omitempty"  json:"character,omitempty"`
	CreditID           string  `bson:"credit_id"            json:"credit_id"`
	Order              *int    `bson:"order,omitempty"      json:"order,omitempty"`
	Department         *string `bson:"department,omitempty" json:"department,omitempty"`
	Job                *string `bson:"job,omitempty"        json:"job,omitempty"`
}

type ExternalIDs struct {
	ImdbID      string  `bson:"imdb_id"      json:"imdb_id"`
	WikidataID  string  `bson:"wikidata_id"  json:"wikidata_id"`
	FacebookID  *string `bson:"facebook_id"  json:"facebook_id"`
	InstagramID *string `bson:"instagram_id" json:"instagram_id"`
	TwitterID   *string `bson:"twitter_id"   json:"twitter_id"`
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

type ReleaseDates struct {
	Results []ReleaseDatesResult `bson:"results" json:"results"`
}

type ReleaseDatesResult struct {
	ISO31661     string        `bson:"iso_3166_1"    json:"iso_3166_1"`
	ReleaseDates []ReleaseDate `bson:"release_dates" json:"release_dates"`
}

type ReleaseDate struct {
	Certification string   `bson:"certification" json:"certification"`
	Descriptors   []string `bson:"descriptors"   json:"descriptors"`
	ISO6391       string   `bson:"iso_639_1"     json:"iso_639_1"`
	Note          string   `bson:"note"          json:"note"`
	ReleaseDate   string   `bson:"release_date"  json:"release_date"`
	Type          int      `bson:"type"          json:"type"`
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

// ReconcileResult reports the outcome of ReconcilePopularity.
type ReconcileResult struct {
	Scanned       int64              // movie docs examined
	Updated       int64              // popularity values changed
	Deleted       int64              // movie docs removed (cascaded to movie_user)
	SkippedDelete bool               // true when the safety guard aborted the delete phase
	ExistingIDs   map[int64]struct{} // every id present in the collection (pre-delete)
}
