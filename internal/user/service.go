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

	identity := Identity{Provider: parseProvider(auth0ID), Auth0ID: auth0ID}
	if err := s.repo.UpsertNewFromAuth0(ctx, identity, profile.Email, Profile{Name: profile.Name, Avatar: profile.Avatar}); err != nil {
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

	existing, err := s.repo.FindByAuth0ID(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}

	if existing == nil {
		identity := Identity{Provider: parseProvider(auth0ID), Auth0ID: auth0ID}
		if err := s.repo.UpsertNewFromAuth0(ctx, identity, profile.Email, userProfile); err != nil {
			return nil, fmt.Errorf("user service: sync from auth0: %w", err)
		}
	} else if err := s.repo.RefreshProfile(ctx, auth0ID, profile.Email, userProfile); err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}

	synced, err := s.repo.FindByAuth0ID(ctx, auth0ID)
	if err != nil {
		return nil, fmt.Errorf("user service: sync from auth0: %w", err)
	}
	return synced, nil
}

// parseProvider extracts the Auth0 connection/provider from a sub claim of
// the form "provider|id" (e.g. "line|xxx", "auth0|xxx", "google-oauth2|xxx").
func parseProvider(auth0ID string) string {
	provider, _, found := strings.Cut(auth0ID, "|")
	if !found {
		return auth0ID
	}
	return provider
}
