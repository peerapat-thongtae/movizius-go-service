// Package tmdb provides an HTTP client for the TMDB v3 API.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.themoviedb.org/3"

// ErrNotFound is returned when TMDB responds with 404 (source deleted or never existed).
var ErrNotFound = fmt.Errorf("tmdb: not found")

// Client is a TMDB API client authenticated via a Bearer access token.
type Client struct {
	http        *http.Client
	accessToken string
}

// New constructs a TMDB Client using the provided read-access token.
func New(accessToken string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 10 * time.Second},
		accessToken: accessToken,
	}
}

// GetMovieDetail fetches /movie/{id} from TMDB.
// appendKeys is the comma-separated list of append_to_response keys, e.g.
// "casts,videos,watch/providers,release_dates,external_ids".
// The response is decoded into the provided target (must be a pointer to a struct
// whose JSON tags match TMDB's response keys).
func (c *Client) GetMovieDetail(ctx context.Context, id int64, appendKeys string, target any) error {
	url := fmt.Sprintf("%s/movie/%d?append_to_response=%s&language=en-US", baseURL, id, appendKeys)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: request /movie/%d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("tmdb: /movie/%d: %w", id, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /movie/%d returned status %d", id, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /movie/%d: %w", id, err)
	}
	return nil
}

// GetTVSeason fetches /tv/{id}/season/{season} from TMDB.
func (c *Client) GetTVSeason(ctx context.Context, tvID int64, season int, target any) error {
	url := fmt.Sprintf("%s/tv/%d/season/%d?language=en-US", baseURL, tvID, season)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: request /tv/%d/season/%d: %w", tvID, season, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /tv/%d/season/%d returned status %d", tvID, season, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /tv/%d/season/%d: %w", tvID, season, err)
	}
	return nil
}

// GetTVEpisode fetches /tv/{id}/season/{season}/episode/{episode} from TMDB.
func (c *Client) GetTVEpisode(ctx context.Context, tvID int64, season, episode int, target any) error {
	url := fmt.Sprintf("%s/tv/%d/season/%d/episode/%d?language=en-US", baseURL, tvID, season, episode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: request /tv/%d/season/%d/episode/%d: %w", tvID, season, episode, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /tv/%d/season/%d/episode/%d returned status %d", tvID, season, episode, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /tv/%d/season/%d/episode/%d: %w", tvID, season, episode, err)
	}
	return nil
}

// TrendingPage is the TMDB paginated response for trending endpoints.
type TrendingPage struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []struct {
		ID int64 `json:"id"`
	} `json:"results"`
}

// GetTrending fetches /trending/{mediaType}/{timeWindow} from TMDB.
// mediaType is "movie" or "tv"; timeWindow is "day" or "week".
func (c *Client) GetTrending(ctx context.Context, mediaType, timeWindow string, page int) (*TrendingPage, error) {
	url := fmt.Sprintf("%s/trending/%s/%s?page=%d&language=en-US", baseURL, mediaType, timeWindow, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb: request /trending/%s/%s: %w", mediaType, timeWindow, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb: /trending/%s/%s returned status %d", mediaType, timeWindow, resp.StatusCode)
	}

	var result TrendingPage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tmdb: decode /trending/%s/%s: %w", mediaType, timeWindow, err)
	}
	return &result, nil
}

// ChangesPage is the TMDB paginated response for the changes endpoints.
type ChangesPage struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []struct {
		ID    int64 `json:"id"`
		Adult bool  `json:"adult"`
	} `json:"results"`
}

// GetChanges fetches /movie/changes or /tv/changes from TMDB for the given date window.
// mediaType is "movie" or "tv". startDate/endDate are "YYYY-MM-DD" (endDate is optional).
func (c *Client) GetChanges(ctx context.Context, mediaType, startDate, endDate string, page int) (*ChangesPage, error) {
	url := fmt.Sprintf("%s/%s/changes?start_date=%s&page=%d", baseURL, mediaType, startDate, page)
	if endDate != "" {
		url += "&end_date=" + endDate
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb: request /%s/changes: %w", mediaType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb: /%s/changes returned status %d", mediaType, resp.StatusCode)
	}

	var result ChangesPage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tmdb: decode /%s/changes: %w", mediaType, err)
	}
	return &result, nil
}

// SearchPage is the TMDB paginated response for search endpoints.
type SearchPage[T any] struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []T `json:"results"`
}

// SearchMovie calls /3/search/movie and decodes the result into target.
func (c *Client) SearchMovie(ctx context.Context, query string, page int, target any) error {
	endpoint := fmt.Sprintf("%s/search/movie?query=%s&page=%d&language=en-US", baseURL, url.QueryEscape(query), page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: request /search/movie: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /search/movie returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /search/movie: %w", err)
	}
	return nil
}

// SearchTV calls /3/search/tv and decodes the result into target.
func (c *Client) SearchTV(ctx context.Context, query string, page int, target any) error {
	endpoint := fmt.Sprintf("%s/search/tv?query=%s&page=%d&language=en-US", baseURL, url.QueryEscape(query), page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: request /search/tv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /search/tv returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /search/tv: %w", err)
	}
	return nil
}

// GetTVDetail fetches /tv/{id} from TMDB.
// appendKeys is the comma-separated list of append_to_response keys, e.g.
// "credits,videos,watch/providers,external_ids".
func (c *Client) GetTVDetail(ctx context.Context, id int64, appendKeys string, target any) error {
	url := fmt.Sprintf("%s/tv/%d?append_to_response=%s&language=en-US", baseURL, id, appendKeys)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: request /tv/%d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("tmdb: /tv/%d: %w", id, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /tv/%d returned status %d", id, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /tv/%d: %w", id, err)
	}
	return nil
}
