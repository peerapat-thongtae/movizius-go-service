// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"os"
)

// Config holds all application configuration loaded from the environment.
type Config struct {
	MongoURI                     string
	UpstashURL                   string
	UpstashToken                 string
	Auth0IssuerURL               string
	Auth0Audience                string
	Auth0Domain                  string
	Auth0ClientID                string
	Auth0ClientSecret            string
	Port                         string
	FirebaseServiceAccountBase64 string
	TMDBAccessToken              string
	NodeEnv                      string
}

// IsDevelopment reports whether the service is running in the development environment.
func (c *Config) IsDevelopment() bool {
	return c.NodeEnv == "development"
}

// Load reads required environment variables and returns a Config.
// Returns an error if any required variable is missing or empty.
func Load() (*Config, error) {
	cfg := &Config{
		MongoURI:                     os.Getenv("MONGO_URI"),
		UpstashURL:                   os.Getenv("UPSTASH_REDIS_REST_URL"),
		UpstashToken:                 os.Getenv("UPSTASH_REDIS_REST_TOKEN"),
		Auth0IssuerURL:               os.Getenv("AUTH0_ISSUER_URL"),
		Auth0Audience:                os.Getenv("AUTH0_AUDIENCE"),
		Auth0Domain:                  os.Getenv("AUTH0_DOMAIN"),
		Auth0ClientID:                os.Getenv("AUTH0_CLIENT_ID"),
		Auth0ClientSecret:            os.Getenv("AUTH0_CLIENT_SECRET"),
		Port:                         os.Getenv("PORT"),
		FirebaseServiceAccountBase64: os.Getenv("FIREBASE_SERVICE_ACCOUNT_BASE64"),
		TMDBAccessToken:              os.Getenv("TMDB_API_READ_ACCESS_TOKEN"),
		NodeEnv:                      os.Getenv("NODE_ENV"),
	}

	if cfg.MongoURI == "" {
		return nil, errors.New("MONGO_URI is required")
	}
	if cfg.UpstashURL == "" {
		return nil, errors.New("UPSTASH_REDIS_REST_URL is required")
	}
	if cfg.UpstashToken == "" {
		return nil, errors.New("UPSTASH_REDIS_REST_TOKEN is required")
	}
	if cfg.Auth0IssuerURL == "" {
		return nil, errors.New("AUTH0_ISSUER_URL is required")
	}
	if cfg.Auth0Audience == "" {
		return nil, errors.New("AUTH0_AUDIENCE is required")
	}
	if cfg.Auth0Domain == "" {
		return nil, errors.New("AUTH0_DOMAIN is required")
	}
	if cfg.Auth0ClientID == "" {
		return nil, errors.New("AUTH0_CLIENT_ID is required")
	}
	if cfg.Auth0ClientSecret == "" {
		return nil, errors.New("AUTH0_CLIENT_SECRET is required")
	}
	if cfg.FirebaseServiceAccountBase64 == "" {
		return nil, errors.New("FIREBASE_SERVICE_ACCOUNT_BASE64 is required")
	}
	if cfg.TMDBAccessToken == "" {
		return nil, errors.New("TMDB_ACCESS_TOKEN is required")
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	return cfg, nil
}
