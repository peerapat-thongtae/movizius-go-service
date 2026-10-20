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

// PruneUnacceptableUntracked deletes movie documents that fail the acceptability filter and are not
// tracked by any user. When dryRun is true it returns the candidates without deleting them.
func (s *MovieSyncService) PruneUnacceptableUntracked(ctx context.Context, dryRun bool) ([]PruneCandidate, error) {
	return s.repo.PruneUnacceptableUntracked(ctx, dryRun)
}

// Sync fetches TMDB detail for each ID in parallel and upserts into the movie collection.
// When skipAcceptable is true, the acceptability filter is bypassed. User-tracked movies
// bypass it as well, so their cached metadata remains available regardless of catalog quality.
func (s *MovieSyncService) Sync(ctx context.Context, ids []int64, skipAcceptable bool) error {
	if len(ids) == 0 {
		return nil
	}
	trackedIDs, err := s.repo.FindTrackedIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("movie sync: find tracked ids: %w", err)
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
			// This TMDB keyword is denied regardless of catalog acceptability,
			// explicit skip flags, or whether a user tracks the title.
			if hasDeniedKeyword(detail.Keywords) {
				if err := s.repo.DeleteByTMDBID(ctx, movieID); err != nil {
					errs[idx] = fmt.Errorf("delete denied movie %d: %w", movieID, err)
				}
				return
			}
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
			}
			_, isTracked := trackedIDs[movieID]
			if !skipAcceptable && !isTracked && !isAcceptableMovie(detail) {
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
