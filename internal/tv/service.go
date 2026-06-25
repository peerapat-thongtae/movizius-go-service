package tv

import (
	"context"
	"fmt"
)

// TVService holds the business logic for the TV feature.
type TVService struct {
	repo TVRepository
}

// NewService constructs a TVService.
func NewService(repo TVRepository) *TVService {
	return &TVService{repo: repo}
}

// GetStates returns all TV tracking records for the given user.
func (s *TVService) GetStates(ctx context.Context, userID string) ([]TVUser, error) {
	states, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get states: %w", err)
	}
	return states, nil
}
