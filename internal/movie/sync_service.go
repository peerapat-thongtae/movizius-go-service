package movie

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// MovieSyncService fetches TMDB metadata for a set of movie IDs and upserts them into MongoDB.
type MovieSyncService struct {
	repo MovieRepository
	tmdb *tmdb.Client
}

// NewSyncService constructs a MovieSyncService.
func NewSyncService(repo MovieRepository, tmdb *tmdb.Client) *MovieSyncService {
	return &MovieSyncService{repo: repo, tmdb: tmdb}
}

// Sync fetches TMDB detail for each ID in parallel and upserts into the movie collection.
// When skipAcceptable is true, the acceptability filter is bypassed.
func (s *MovieSyncService) Sync(ctx context.Context, ids []int64, skipAcceptable bool) error {
	if len(ids) == 0 {
		return nil
	}

	errs := make([]error, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, movieID int64) {
			defer wg.Done()
			var detail MovieResponse
			if err := s.tmdb.GetMovieDetail(ctx, movieID, appendToResponse, &detail); err != nil {
				if errors.Is(err, tmdb.ErrNotFound) {
					if delErr := s.repo.DeleteByTMDBID(ctx, movieID); delErr != nil {
						errs[idx] = fmt.Errorf("delete missing movie %d: %w", movieID, delErr)
					}
				}
				return
			}
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
			}
			if !skipAcceptable && !isAcceptableMovie(detail) {
				return
			}
			if err := s.repo.UpsertDetail(ctx, detail); err != nil {
				errs[idx] = fmt.Errorf("upsert movie %d: %w", movieID, err)
			}
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return fmt.Errorf("movie sync: %w", err)
		}
	}
	return nil
}
