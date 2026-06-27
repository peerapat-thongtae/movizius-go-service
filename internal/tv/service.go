package tv

import (
	"context"
	"fmt"
	"sync"

	"github.com/peera/movizius-go-service/pkg/tmdb"
)

const appendToResponse = "credits,videos,watch/providers,external_ids"

// TVService holds the business logic for the TV feature.
type TVService struct {
	repo TVRepository
	tmdb *tmdb.Client
}

// NewService constructs a TVService.
func NewService(repo TVRepository, tmdb *tmdb.Client) *TVService {
	return &TVService{repo: repo, tmdb: tmdb}
}

// Discover returns a page of TV series enriched with TMDB detail data.
func (s *TVService) Discover(ctx context.Context, userID string, q DiscoverQuery) ([]TVResponse, int, error) {
	ids, total, err := s.repo.DiscoverIDs(ctx, userID, q)
	if err != nil {
		return nil, 0, fmt.Errorf("tv service: discover ids: %w", err)
	}
	if len(ids) == 0 {
		return []TVResponse{}, total, nil
	}

	results := make([]TVResponse, len(ids))
	errs := make([]error, len(ids))

	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, tvID int64) {
			defer wg.Done()
			var detail TVResponse
			if err := s.tmdb.GetTVDetail(ctx, tvID, appendToResponse, &detail); err != nil {
				errs[idx] = fmt.Errorf("tmdb detail for id %d: %w", tvID, err)
				return
			}
			results[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, 0, fmt.Errorf("tv service: enrich from tmdb: %w", err)
		}
	}

	return results, total, nil
}

// GetStates returns aggregated TV tracking records for the given user.
func (s *TVService) GetStates(ctx context.Context, userID string) ([]TVState, error) {
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get states: %w", err)
	}
	return states, nil
}
