package movie

import (
	"context"
	"fmt"
)

// MovieService holds the business logic for the movie feature.
type MovieService struct {
	repo MovieRepository
}

// NewService constructs a MovieService.
func NewService(repo MovieRepository) *MovieService {
	return &MovieService{repo: repo}
}

// GetStates returns all movie tracking records for the given user.
func (s *MovieService) GetStates(ctx context.Context, userID string) ([]MovieUser, error) {
	states, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("movie service: get states: %w", err)
	}
	return states, nil
}
