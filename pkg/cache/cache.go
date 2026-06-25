// Package cache provides a Cache interface and an Upstash Redis REST implementation.
// The REST API requires no TCP connection — each operation is a single HTTP request,
// which is ideal for Vercel serverless functions.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Cache defines the operations available on the cache layer.
type Cache interface {
	// Get retrieves a value. The bool is false when the key does not exist.
	Get(ctx context.Context, key string) (string, bool, error)
	// Set stores a value with an optional TTL (0 = no expiry).
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	// Delete removes a key.
	Delete(ctx context.Context, key string) error
}

// upstash implements Cache using the Upstash Redis REST API.
type upstash struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewUpstash returns a Cache backed by the Upstash Redis REST API.
func NewUpstash(restURL, token string) Cache {
	return &upstash{
		baseURL: strings.TrimRight(restURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

type upstashResponse struct {
	Result any    `json:"result"`
	Error  string `json:"error"`
}

func (u *upstash) do(ctx context.Context, path string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result upstashResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("upstash error: %s", result.Error)
	}

	return result.Result, nil
}

func (u *upstash) Get(ctx context.Context, key string) (string, bool, error) {
	result, err := u.do(ctx, "/get/"+key)
	if err != nil {
		return "", false, err
	}
	if result == nil {
		return "", false, nil
	}
	str, ok := result.(string)
	if !ok {
		return "", false, fmt.Errorf("unexpected result type from cache GET")
	}
	return str, true, nil
}

func (u *upstash) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	path := "/set/" + key + "/" + value
	if ttl > 0 {
		secs := int64(ttl.Seconds())
		if secs < 1 {
			secs = 1
		}
		path += fmt.Sprintf("/ex/%d", secs)
	}
	_, err := u.do(ctx, path)
	return err
}

func (u *upstash) Delete(ctx context.Context, key string) error {
	_, err := u.do(ctx, "/del/"+key)
	return err
}
