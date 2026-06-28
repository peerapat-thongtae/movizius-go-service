// Package tmdb provides an HTTP client for the TMDB v3 API.
package tvmaze

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://api.tvmaze.com"

// Client is a TMDB API client authenticated via a Bearer access token.
type Client struct {
	http *http.Client
}

// New constructs a TMDB Client using the provided read-access token.
func New(accessToken string) *Client {
	return &Client{
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetMovieDetail fetches /movie/{id} from TMDB.
// appendKeys is the comma-separated list of append_to_response keys, e.g.
// "casts,videos,watch/providers,release_dates,external_ids".
// The response is decoded into the provided target (must be a pointer to a struct
// whose JSON tags match TMDB's response keys).
func (c *Client) GetAiringFullSchedule(ctx context.Context, date string, target any) error {
	url := fmt.Sprintf("%s/schedule/full", baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tvmaze: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tvmaze: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tvmaze returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tvmaze: decode: %w", err)
	}
	return nil
}
