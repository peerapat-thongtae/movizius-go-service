// Package auth0 provides a client for the Auth0 Management API.
package auth0

import (
	"context"
	"fmt"
	"strings"

	"github.com/auth0/go-auth0/management"
)

// Identity is one linked Auth0 identity (a user may have several when
// accounts are linked, e.g. "line|xxx" and "google-oauth2|yyy").
type Identity struct {
	Provider string
	Auth0ID  string
}

// Profile is the subset of an Auth0 Management API user record needed to
// sync a local user record.
type Profile struct {
	Email      string
	Name       string
	Avatar     string
	Identities []Identity
}

// Client wraps the Auth0 Management API, authenticated via client credentials.
type Client struct {
	mgmt *management.Management
}

// New authenticates against the Auth0 Management API using the client
// credentials flow. The returned Client is safe for concurrent use and
// should be created once at startup.
//
// domain accepts either a bare domain ("tenant.auth0.com") or a full issuer
// URL ("https://tenant.auth0.com/") — the scheme and trailing slash are
// stripped, since the Management SDK requires a bare domain.
func New(ctx context.Context, domain, clientID, clientSecret string) (*Client, error) {
	domain = normalizeDomain(domain)
	mgmt, err := management.New(domain, management.WithClientCredentials(ctx, clientID, clientSecret))
	if err != nil {
		return nil, fmt.Errorf("auth0: initialize management client: %w", err)
	}
	return &Client{mgmt: mgmt}, nil
}

func normalizeDomain(domain string) string {
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	return strings.TrimRight(domain, "/")
}

// GetProfile fetches the Auth0 user record for the given user ID (the JWT
// `sub` claim) and returns its email, name, picture, and the full list of
// linked identities (Auth0 merges linked accounts into one user record with
// multiple entries in its "identities" array).
func (c *Client) GetProfile(ctx context.Context, userID string) (Profile, error) {
	u, err := c.mgmt.User.Read(ctx, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("auth0: get user %s: %w", userID, err)
	}

	identities := make([]Identity, 0, len(u.Identities))
	for _, ui := range u.Identities {
		identities = append(identities, Identity{
			Provider: ui.GetProvider(),
			Auth0ID:  ui.GetProvider() + "|" + ui.GetUserID(),
		})
	}
	if len(identities) == 0 {
		// Defensive fallback — the Management API should always populate
		// Identities, but if it doesn't, fall back to the requested user ID.
		identities = append(identities, Identity{Provider: providerFromAuth0ID(userID), Auth0ID: userID})
	}

	return Profile{
		Email:      u.GetEmail(),
		Name:       u.GetName(),
		Avatar:     u.GetPicture(),
		Identities: identities,
	}, nil
}

func providerFromAuth0ID(auth0ID string) string {
	provider, _, found := strings.Cut(auth0ID, "|")
	if !found {
		return auth0ID
	}
	return provider
}
