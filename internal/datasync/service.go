package datasync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/tv"
	"github.com/peera/movizius-go-service/pkg/tmdb"
	"github.com/peera/movizius-go-service/pkg/tvmaze"
)

// SyncService orchestrates chunked sync jobs for movie and TV metadata.
type SyncService struct {
	repo      SyncRepository
	movieSync *movie.MovieSyncService
	tvSync    *tv.TVSyncService
	tmdb      *tmdb.Client
	tvmaze    *tvmaze.Client
}

// NewService constructs a SyncService.
func NewService(repo SyncRepository, movieSync *movie.MovieSyncService, tvSync *tv.TVSyncService, tmdbClient *tmdb.Client, tvmazeClient *tvmaze.Client) *SyncService {
	return &SyncService{repo: repo, movieSync: movieSync, tvSync: tvSync, tmdb: tmdbClient, tvmaze: tvmazeClient}
}

// SyncByIDs syncs TMDB metadata for the given list of IDs directly.
func (s *SyncService) SyncByIDs(ctx context.Context, mediaType string, ids []int64) (*SyncResult, error) {
	var err error
	switch mediaType {
	case "movie":
		err = s.movieSync.Sync(ctx, ids)
	case "tv":
		err = s.tvSync.Sync(ctx, ids)
	default:
		return nil, fmt.Errorf("unsupported media type: %s", mediaType)
	}
	if err != nil {
		return nil, err
	}
	return &SyncResult{
		Source:    "by_ids",
		Total:     len(ids),
		Processed: len(ids),
		Status:    StatusCompleted,
	}, nil
}

// SyncFromUserTracked syncs TMDB metadata for IDs in movie_user or tv_user using offset paging.
// Loads the existing job by syncKey; creates a new one if not found yet.
func (s *SyncService) SyncFromUserTracked(ctx context.Context, syncKey, mediaType, frequency string, limit int) (*SyncResult, error) {
	meta, err := s.repo.GetSyncMeta(ctx, syncKey)
	if err != nil && !isNotFound(err) {
		return nil, err
	}

	if isNotFound(err) {
		total, err := s.countIDs(ctx, mediaType)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		meta = &SyncMeta{
			SyncKey:   syncKey,
			Source:    SourceUserTracked,
			MediaType: mediaType,
			Status:    StatusInProgress,
			Meta:      bson.M{"offset": 0, "total": total},
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.repo.CreateSyncMeta(ctx, *meta); err != nil {
			return nil, err
		}
	}

	// Frequency check: if completed, decide whether to re-run or return not_due.
	if meta.Status == StatusCompleted {
		total := int(toInt64(meta.Meta["total"]))
		nextSyncAt, due := nextSyncTime(meta.SyncDate, frequency)
		if !due {
			return &SyncResult{
				SyncKey:    syncKey,
				Source:     SourceUserTracked,
				Frequency:  frequency,
				Total:      total,
				Processed:  total,
				Remaining:  0,
				Status:     StatusNotDue,
				NextSyncAt: nextSyncAt,
			}, nil
		}
		// Period elapsed — reset for a new cycle.
		newTotal, err := s.countIDs(ctx, mediaType)
		if err != nil {
			return nil, err
		}
		resetMeta := bson.M{"offset": 0, "total": newTotal}
		if err := s.repo.UpdateSyncMeta(ctx, syncKey, resetMeta, StatusInProgress, nil); err != nil {
			return nil, err
		}
		meta.Meta = resetMeta
		meta.Status = StatusInProgress
		meta.SyncDate = nil
	}

	offset := int(toInt64(meta.Meta["offset"]))
	total := int(toInt64(meta.Meta["total"]))

	chunk, err := s.getIDsPage(ctx, mediaType, offset, limit)
	if err != nil {
		return nil, err
	}

	if err := s.syncChunk(ctx, mediaType, chunk); err != nil {
		return nil, err
	}

	newOffset := offset + len(chunk)
	status := StatusInProgress
	var syncDate *time.Time
	// Complete when offset reaches total OR fewer items returned than requested (real end).
	if newOffset >= total || len(chunk) < limit {
		status = StatusCompleted
		newOffset = total
		now := time.Now().UTC()
		syncDate = &now
	}

	if err := s.repo.UpdateSyncMeta(ctx, syncKey, bson.M{"offset": newOffset, "total": total}, status, syncDate); err != nil {
		return nil, err
	}

	nextSyncAt, _ := nextSyncTime(syncDate, frequency)
	return &SyncResult{
		SyncKey:    syncKey,
		Source:     SourceUserTracked,
		Frequency:  frequency,
		Total:      total,
		Processed:  newOffset,
		Remaining:  total - newOffset,
		Status:     status,
		NextSyncAt: nextSyncAt,
	}, nil
}

// SyncFromTMDBTrending syncs TMDB trending items page by page.
// limit controls how many TMDB pages to fetch per call (default 1 = ~20 items).
// timeWindow is "day" or "week".
func (s *SyncService) SyncFromTMDBTrending(ctx context.Context, syncKey, mediaType, timeWindow, frequency string, limit int) (*SyncResult, error) {
	m, err := s.repo.GetSyncMeta(ctx, syncKey)
	if err != nil && !isNotFound(err) {
		return nil, err
	}

	if isNotFound(err) {
		page, err := s.tmdb.GetTrending(ctx, mediaType, timeWindow, 1)
		if err != nil {
			return nil, fmt.Errorf("datasync: trending page 1: %w", err)
		}
		if err := s.syncChunk(ctx, mediaType, extractIDs(page)); err != nil {
			return nil, err
		}

		totalPages := page.TotalPages
		currentPage := 2
		status := StatusInProgress
		var syncDate *time.Time
		if totalPages <= 1 {
			currentPage = totalPages + 1
			status = StatusCompleted
			now := time.Now().UTC()
			syncDate = &now
		}

		now := time.Now().UTC()
		newMeta := SyncMeta{
			SyncKey:   syncKey,
			Source:    SourceTMDBTrending,
			MediaType: mediaType,
			Status:    status,
			SyncDate:  syncDate,
			Meta: bson.M{
				"time_window":  timeWindow,
				"current_page": currentPage,
				"total_pages":  totalPages,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.repo.CreateSyncMeta(ctx, newMeta); err != nil {
			return nil, err
		}
		nextSyncAt, _ := nextSyncTime(syncDate, frequency)
		return &SyncResult{
			SyncKey:    syncKey,
			Source:     SourceTMDBTrending,
			Frequency:  frequency,
			Total:      totalPages,
			Processed:  1,
			Remaining:  totalPages - 1,
			Status:     status,
			NextSyncAt: nextSyncAt,
		}, nil
	}

	// Frequency check.
	if m.Status == StatusCompleted {
		tp := int(toInt64(m.Meta["total_pages"]))
		nextSyncAt, due := nextSyncTime(m.SyncDate, frequency)
		if !due {
			return &SyncResult{
				SyncKey:    syncKey,
				Source:     SourceTMDBTrending,
				Frequency:  frequency,
				Total:      tp,
				Processed:  tp,
				Remaining:  0,
				Status:     StatusNotDue,
				NextSyncAt: nextSyncAt,
			}, nil
		}
		// Period elapsed — reset for a new cycle.
		tw, _ := m.Meta["time_window"].(string)
		resetMeta := bson.M{"time_window": tw, "current_page": 1, "total_pages": tp}
		if err := s.repo.UpdateSyncMeta(ctx, syncKey, resetMeta, StatusInProgress, nil); err != nil {
			return nil, err
		}
		m.Meta = resetMeta
		m.Status = StatusInProgress
		m.SyncDate = nil
	}

	currentPage := int(toInt64(m.Meta["current_page"]))
	totalPages := int(toInt64(m.Meta["total_pages"]))
	timeWindow, _ = m.Meta["time_window"].(string)

	end := currentPage + limit
	if end > totalPages+1 {
		end = totalPages + 1
	}

	for p := currentPage; p < end; p++ {
		page, err := s.tmdb.GetTrending(ctx, mediaType, timeWindow, p)
		if err != nil {
			return nil, fmt.Errorf("datasync: trending page %d: %w", p, err)
		}
		if err := s.syncChunk(ctx, mediaType, extractIDs(page)); err != nil {
			return nil, err
		}
	}

	newPage := end
	status := StatusInProgress
	var syncDate *time.Time
	if newPage > totalPages {
		status = StatusCompleted
		now := time.Now().UTC()
		syncDate = &now
	}

	updatedMeta := bson.M{
		"time_window":  timeWindow,
		"current_page": newPage,
		"total_pages":  totalPages,
	}
	if err := s.repo.UpdateSyncMeta(ctx, syncKey, updatedMeta, status, syncDate); err != nil {
		return nil, err
	}

	processed := newPage - 1
	if processed > totalPages {
		processed = totalPages
	}
	nextSyncAt, _ := nextSyncTime(syncDate, frequency)
	return &SyncResult{
		SyncKey:    syncKey,
		Source:     SourceTMDBTrending,
		Frequency:  frequency,
		Total:      totalPages,
		Processed:  processed,
		Remaining:  totalPages - processed,
		Status:     status,
		NextSyncAt: nextSyncAt,
	}, nil
}

// nextSyncTime returns the time of the next sync and whether the period has elapsed.
func nextSyncTime(syncDate *time.Time, frequency string) (*time.Time, bool) {
	if syncDate == nil {
		return nil, true
	}
	var d time.Duration
	switch frequency {
	case FrequencyDaily:
		d = 24 * time.Hour
	case FrequencyMonthly:
		d = 30 * 24 * time.Hour
	default: // weekly
		d = 7 * 24 * time.Hour
	}
	next := syncDate.Add(d)
	return &next, time.Now().After(next)
}

func (s *SyncService) countIDs(ctx context.Context, mediaType string) (int, error) {
	switch mediaType {
	case "movie":
		return s.repo.CountMovieIDs(ctx)
	case "tv":
		return s.repo.CountTVIDs(ctx)
	default:
		return 0, fmt.Errorf("datasync: unknown media type %q", mediaType)
	}
}

func (s *SyncService) getIDsPage(ctx context.Context, mediaType string, offset, limit int) ([]int64, error) {
	switch mediaType {
	case "movie":
		return s.repo.GetMovieIDsPage(ctx, offset, limit)
	case "tv":
		return s.repo.GetTVIDsPage(ctx, offset, limit)
	default:
		return nil, fmt.Errorf("datasync: unknown media type %q", mediaType)
	}
}

func (s *SyncService) syncChunk(ctx context.Context, mediaType string, ids []int64) error {
	switch mediaType {
	case "movie":
		return s.movieSync.Sync(ctx, ids)
	case "tv":
		return s.tvSync.Sync(ctx, ids)
	default:
		return fmt.Errorf("datasync: unknown media type %q", mediaType)
	}
}

func extractIDs(page *tmdb.TrendingPage) []int64 {
	ids := make([]int64, 0, len(page.Results))
	for _, r := range page.Results {
		ids = append(ids, r.ID)
	}
	return ids
}

func isNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}

// SyncTVMazeSchedule fetches the full TVMaze airing schedule and updates
// next_episode_to_air.air_date in the tv collection for matching records.
func (s *SyncService) SyncTVMazeSchedule(ctx context.Context) (*SyncResult, error) {
	var entries []tvmaze.ScheduleEntry
	if err := s.tvmaze.GetAiringFullSchedule(ctx, "", &entries); err != nil {
		return nil, fmt.Errorf("datasync: tvmaze schedule: %w", err)
	}

	updates := make([]tv.NextEpisodeAirDateUpdate, 0, len(entries))
	for _, e := range entries {
		if e.Embedded.Show.Externals.Imdb == nil || *e.Embedded.Show.Externals.Imdb == "" {
			continue
		}
		updates = append(updates, tv.NextEpisodeAirDateUpdate{
			ImdbID:        *e.Embedded.Show.Externals.Imdb,
			SeasonNumber:  e.Season,
			EpisodeNumber: e.Number,
			AirDate:       e.Airstamp.UTC().Format(time.RFC3339),
		})
	}

	if err := s.tvSync.UpdateNextEpisodeAirDates(ctx, updates); err != nil {
		return nil, fmt.Errorf("datasync: tvmaze schedule update: %w", err)
	}

	return &SyncResult{
		SyncKey:   "sync_tv_tvmaze_schedule",
		Source:    "tvmaze",
		Total:     len(entries),
		Processed: len(updates),
		Remaining: 0,
		Status:    StatusCompleted,
	}, nil
}

// CleanupMovieFields removes stale keys from all movie documents that are no longer in the DB model.
func (s *SyncService) CleanupMovieFields(ctx context.Context) (int64, error) {
	return s.repo.CleanupMovieFields(ctx)
}

// CleanupTVFields removes stale keys from all tv documents that are no longer in the DB model.
func (s *SyncService) CleanupTVFields(ctx context.Context) (int64, error) {
	return s.repo.CleanupTVFields(ctx)
}

// toInt64 converts BSON-decoded numeric types to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}
