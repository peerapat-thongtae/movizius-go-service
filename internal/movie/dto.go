package movie

import (
	"net/http"
	"strconv"
	"strings"
)

// DiscoverQuery holds parsed query params for the movie discover endpoint.
type DiscoverQuery struct {
	Page                 int
	SortBy               string
	WithGenres           []int64
	WithoutGenres        []int64
	PrimaryReleaseYear   int
	ReleaseDateGte       string
	ReleaseDateLte       string
	VoteAverageGte       float64
	VoteAverageLte       float64
	VoteCountGte         int64
	WithOriginalLanguage string
	IncludeAdult         bool
	Softcore             *bool
	WithWatchProviders    []int64
	WatchRegion           string
	WithAccountStatus    []string
	WithoutAccountStatus []string
	WithReleaseDateGte   string
	WithReleaseDateLte   string
}

// UpsertStateRequest is the body for POST /movie.
type UpsertStateRequest struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

// discoverQueryFromRequest parses DiscoverQuery from the request's URL query params.
func discoverQueryFromRequest(r *http.Request) DiscoverQuery {
	q := r.URL.Query()

	dq := DiscoverQuery{
		Page:                 intParam(q.Get("page"), 1),
		SortBy:               stringParam(q.Get("sort_by"), "popularity.desc"),
		WithGenres:           int64ListParam(q.Get("with_genres")),
		WithoutGenres:        int64ListParam(q.Get("without_genres")),
		PrimaryReleaseYear:   intParam(q.Get("primary_release_year"), 0),
		ReleaseDateGte:       q.Get("release_date.gte"),
		ReleaseDateLte:       q.Get("release_date.lte"),
		VoteAverageGte:       float64Param(q.Get("vote_average.gte"), 0),
		VoteAverageLte:       float64Param(q.Get("vote_average.lte"), 0),
		VoteCountGte:         int64Param(q.Get("vote_count.gte"), 0),
		WithOriginalLanguage: q.Get("with_original_language"),
		IncludeAdult:         q.Get("include_adult") == "true",
		WithWatchProviders:    int64ListParam(q.Get("with_watch_providers")),
		WatchRegion:           strings.ToUpper(q.Get("watch_region")),
		WithAccountStatus:    stringListParam(q.Get("with_account_status")),
		WithoutAccountStatus: stringListParam(q.Get("without_account_status")),
		WithReleaseDateGte:   q.Get("with_release_date.gte"),
		WithReleaseDateLte:   q.Get("with_release_date.lte"),
	}

	if raw := q.Get("softcore"); raw != "" {
		v := raw == "true"
		dq.Softcore = &v
	}

	if dq.Page < 1 {
		dq.Page = 1
	}

	return dq
}

func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func int64Param(s string, def int64) int64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

func float64Param(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func stringParam(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func stringListParam(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func int64ListParam(s string) []int64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}
