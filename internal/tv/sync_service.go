package tv

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// TVSyncService fetches TMDB metadata for a set of TV IDs and upserts them into MongoDB.
type TVSyncService struct {
	repo TVRepository
	tmdb *tmdb.Client
}

// NewSyncService constructs a TVSyncService.
func NewSyncService(repo TVRepository, tmdb *tmdb.Client) *TVSyncService {
	return &TVSyncService{repo: repo, tmdb: tmdb}
}

// UpdateNextEpisodeAirDates patches next_episode_to_air.air_date for matched records.
func (s *TVSyncService) UpdateNextEpisodeAirDates(ctx context.Context, updates []NextEpisodeAirDateUpdate) error {
	return s.repo.UpdateNextEpisodeAirDates(ctx, updates)
}

// PruneUnacceptableUntracked deletes tv documents that fail the acceptability filter and are not
// tracked by any user. When dryRun is true it returns the candidates without deleting them.
func (s *TVSyncService) PruneUnacceptableUntracked(ctx context.Context, dryRun bool) ([]PruneCandidate, error) {
	return s.repo.PruneUnacceptableUntracked(ctx, dryRun)
}

// Sync fetches TMDB detail for each ID in parallel and upserts into the tv collection.
// When skipAcceptable is true, the acceptability filter is bypassed. User-tracked series
// bypass it as well, so their cached metadata remains available regardless of catalog quality.
func (s *TVSyncService) Sync(ctx context.Context, ids []int64, skipAcceptable bool) error {
	if len(ids) == 0 {
		return nil
	}
	trackedIDs, err := s.repo.FindTrackedIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("tv sync: find tracked ids: %w", err)
	}

	errs := make([]error, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, tvID int64) {
			defer wg.Done()
			var detail TVResponse
			if err := s.tmdb.GetTVDetail(ctx, tvID, appendToResponse, &detail); err != nil {
				if errors.Is(err, tmdb.ErrNotFound) {
					if delErr := s.repo.DeleteByTMDBID(ctx, tvID); delErr != nil {
						errs[idx] = fmt.Errorf("delete missing tv %d: %w", tvID, delErr)
					}
				}
				return
			}
			_, isTracked := trackedIDs[tvID]
			// Sparse or denied keywords keep untracked series out of the catalog,
			// but never remove a title a user explicitly chose to track.
			if !isTracked && hasDeniedKeyword(detail.Keywords) {
				if err := s.repo.DeleteByTMDBID(ctx, tvID); err != nil {
					errs[idx] = fmt.Errorf("delete denied tv %d: %w", tvID, err)
				}
				return
			}
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
			}
			if !skipAcceptable && !isTracked && !isAcceptableTV(detail) {
				return
			}
			if err := s.repo.UpsertDetail(ctx, detail); err != nil {
				errs[idx] = fmt.Errorf("upsert tv %d: %w", tvID, err)
			}
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return fmt.Errorf("tv sync: %w", err)
		}
	}
	return nil
}
