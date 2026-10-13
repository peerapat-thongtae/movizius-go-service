// Package person provides person-profile data backed by TMDB and the local catalog.
package person

// Detail is the subset of a TMDB person detail needed by the web client.
type Detail struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	ProfilePath        *string `json:"profile_path"`
	Biography          string  `json:"biography"`
	KnownForDepartment string  `json:"known_for_department"`
	Birthday           *string `json:"birthday"`
	Deathday           *string `json:"deathday"`
	PlaceOfBirth       *string `json:"place_of_birth"`
}

// Credit is a movie or TV credit from TMDB's combined credits response.
type Credit struct {
	ID           int64   `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title,omitempty"`
	Name         string  `json:"name,omitempty"`
	PosterPath   *string `json:"poster_path"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	FirstAirDate string  `json:"first_air_date,omitempty"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int64   `json:"vote_count"`
	Character    string  `json:"character,omitempty"`
	Job          string  `json:"job,omitempty"`
	Department   string  `json:"department,omitempty"`
}

// Credits holds cast and crew credits.
type Credits struct {
	Cast []Credit `json:"cast"`
	Crew []Credit `json:"crew"`
}

// Response is the person detail response exposed by the API.
type Response struct {
	Person  Detail  `json:"person"`
	Credits Credits `json:"credits"`
}

// SearchResult is the subset of a TMDB person search result needed by the web client.
type SearchResult struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	ProfilePath        *string `json:"profile_path"`
	KnownForDepartment string  `json:"known_for_department"`
	Popularity         float64 `json:"popularity"`
}
