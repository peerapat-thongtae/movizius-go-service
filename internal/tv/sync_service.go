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

// Sync fetches TMDB detail for each ID in parallel and upserts into the tv collection.
func (s *TVSyncService) Sync(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
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
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
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
