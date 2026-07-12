package user

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/peera/movizius-go-service/pkg/auth0"
)

// UserService holds the business logic for syncing users from Auth0.
type UserService struct {
	repo  UserRepository
	auth0 *auth0.Client
	log   *slog.Logger
}

// NewService constructs a UserService.
func NewService(repo UserRepository, auth0Client *auth0.Client, log *slog.Logger) *UserService {
	return &UserService{repo: repo, auth0: auth0Client, log: log}
}

// EnsureUser is the lazy-sync entrypoint: it refreshes last-login for an
// already-known user (no Auth0 call), or creates a new record by fetching
// the profile from the Auth0 Management API on first login.
func (s *UserService) EnsureUser(ctx context.Context, auth0ID string) (*User, error) {
	existing, err := s.repo.FindByAuth0ID(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: ensure user: %w", err)
	}
	if existing != nil {
		if err := s.repo.TouchLastLogin(ctx, auth0ID); err != nil {
			return nil, fmt.Errorf("user service: ensure user: %w", err)
		}
		refreshed, err := s.repo.FindByAuth0ID(ctx, auth0ID)
		if err != nil {
			return nil, fmt.Errorf("user service: ensure user: %w", err)
		}
		return refreshed, nil
	}

	profile, err := s.auth0.GetProfile(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: ensure user: %w", err)
	}

	identities := toIdentities(auth0ID, profile.Identities)
	if err := s.repo.UpsertNewFromAuth0(ctx, auth0ID, identities, profile.Email, Profile{Name: profile.Name, Avatar: profile.Avatar}); err != nil {
		return nil, fmt.Errorf("user service: ensure user: %w", err)
	}

	created, err := s.repo.FindByAuth0ID(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: ensure user: %w", err)
	}
	return created, nil
}

// SyncFromAuth0 force-refreshes the profile fields from the Auth0 Management
// API, creating the user record first if it doesn't exist yet.
func (s *UserService) SyncFromAuth0(ctx context.Context, auth0ID string) (*User, error) {
	profile, err := s.auth0.GetProfile(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}
	userProfile := Profile{Name: profile.Name, Avatar: profile.Avatar}
	identities := toIdentities(auth0ID, profile.Identities)

	existing, err := s.repo.FindByAuth0ID(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}

	if existing == nil {
		if err := s.repo.UpsertNewFromAuth0(ctx, auth0ID, identities, profile.Email, userProfile); err != nil {
			return nil, fmt.Errorf("user service: sync from auth0: %w", err)
		}
	} else if err := s.repo.RefreshProfile(ctx, auth0ID, identities, profile.Email, userProfile); err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}

	synced, err := s.repo.FindByAuth0ID(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}
	return synced, nil
}

// toIdentities converts the Auth0 client's Identity list to this package's
// storage shape, and guarantees the literal sub claim from the token
// (authID) is present verbatim in the result — even if it doesn't exactly
// match any provider|userID pair reconstructed from the Management API
// response. This keeps identities.auth0Id lookups (TouchLastLogin,
// FindByAuth0ID) reliable regardless of any formatting drift between the
// token issuer and the Management API.
func toIdentities(authID string, src []auth0.Identity) []Identity {
	identities := make([]Identity, 0, len(src)+1)
	found := false
	for _, id := range src {
		identities = append(identities, Identity{Provider: id.Provider, Auth0ID: id.Auth0ID})
		if id.Auth0ID == authID {
			found = true
		}
	}
	if !found {
		provider, _, _ := strings.Cut(authID, "|")
		identities = append(identities, Identity{Provider: provider, Auth0ID: authID})
	}
	return identities
}
