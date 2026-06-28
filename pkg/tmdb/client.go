// Package tmdb provides an HTTP client for the TMDB v3 API.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://api.themoviedb.org/3"

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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: /tv/%d returned status %d", id, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb: decode /tv/%d: %w", id, err)
	}
	return nil
}
