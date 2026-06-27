package movie

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/peera/movizius-go-service/pkg/tmdb"
	"golang.org/x/sync/semaphore"
)

const (
	appendToResponse     = "casts,videos,watch/providers,release_dates,external_ids"
	tmdbConcurrencyLimit = 5
)

// MovieService holds the business logic for the movie feature.
type MovieService struct {
	repo MovieRepository
	tmdb *tmdb.Client
}

// NewService constructs a MovieService.
func NewService(repo MovieRepository, tmdb *tmdb.Client) *MovieService {
	return &MovieService{repo: repo, tmdb: tmdb}
}

// GetStates returns all movie tracking records for the given user.
func (s *MovieService) GetStates(ctx context.Context, userID string) ([]MovieUser, error) {
	states, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("movie service: get states: %w", err)
	}
	for i := range states {
		states[i].AccountStatus = accountStatus(states[i].WatchedAt)
	}
	return states, nil
}

// Discover returns a page of movies enriched with TMDB detail data.
func (s *MovieService) Discover(ctx context.Context, userID string, q DiscoverQuery) ([]MovieResponse, int, error) {
	ids, total, err := s.repo.DiscoverIDs(ctx, userID, q)
	println("results", ids, total)
	if err != nil {
		return nil, 0, fmt.Errorf("movie service: discover ids: %w", err)
	}
	if len(ids) == 0 {
		return []MovieResponse{}, total, nil
	}

	results := make([]MovieResponse, len(ids))
	errs := make([]error, len(ids))

	sem := semaphore.NewWeighted(tmdbConcurrencyLimit)
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, movieID int64) {
			defer wg.Done()
			if err := sem.Acquire(ctx, 1); err != nil {
				errs[idx] = err
				return
			}
			defer sem.Release(1)

			var detail MovieResponse
			if err := s.tmdb.GetMovieDetail(ctx, movieID, appendToResponse, &detail); err != nil {
				errs[idx] = fmt.Errorf("tmdb detail for id %d: %w", movieID, err)
				return
			}
			results[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, 0, fmt.Errorf("movie service: enrich from tmdb: %w", err)
		}
	}

	return results, total, nil
}

func accountStatus(watchedAt *time.Time) *string {
	if watchedAt != nil {
		s := "watched"
		return &s
	}
	s := "watchlist"
	return &s
}
