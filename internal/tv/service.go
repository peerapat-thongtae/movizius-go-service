package tv

import (
	"context"
	"fmt"
	"sync"
	"time"

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
			detail.MediaType = "tv"
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
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

// UpsertTVState creates or updates the user's TV tracking record.
// For status="watched" it enumerates all episodes from TMDB and populates episode_watched.
func (s *TVService) UpsertTVState(ctx context.Context, userID string, req UpsertStateRequest) error {
	if req.Status != "watched" && req.Status != "watchlist" {
		return fmt.Errorf("tv service: invalid status %q", req.Status)
	}

	if req.Status == "watchlist" {
		return s.repo.UpsertTVState(ctx, userID, req.ID, nil)
	}

	// Fetch show summary to get the season list.
	var show struct {
		Seasons []struct {
			SeasonNumber int `json:"season_number"`
		} `json:"seasons"`
	}
	if err := s.tmdb.GetTVDetail(ctx, req.ID, "", &show); err != nil {
		return fmt.Errorf("tv service: get tv detail: %w", err)
	}

	// Collect non-special seasons (season_number > 0).
	var seasonNums []int
	for _, s := range show.Seasons {
		if s.SeasonNumber > 0 {
			seasonNums = append(seasonNums, s.SeasonNumber)
		}
	}

	type seasonResult struct {
		episodes []EpisodeWatched
		err      error
	}
	results := make([]seasonResult, len(seasonNums))

	var wg sync.WaitGroup
	for i, sn := range seasonNums {
		wg.Add(1)
		go func(idx, seasonNum int) {
			defer wg.Done()
			var season struct {
				Episodes []struct {
					SeasonNumber  int `json:"season_number"`
					EpisodeNumber int `json:"episode_number"`
				} `json:"episodes"`
			}
			if err := s.tmdb.GetTVSeason(ctx, req.ID, seasonNum, &season); err != nil {
				results[idx].err = err
				return
			}
			now := time.Now().UTC()
			eps := make([]EpisodeWatched, 0, len(season.Episodes))
			for _, ep := range season.Episodes {
				eps = append(eps, EpisodeWatched{
					SeasonNumber:  ep.SeasonNumber,
					EpisodeNumber: ep.EpisodeNumber,
					WatchedAt:     now,
				})
			}
			results[idx].episodes = eps
		}(i, sn)
	}
	wg.Wait()

	var allEpisodes []EpisodeWatched
	for _, r := range results {
		if r.err != nil {
			return fmt.Errorf("tv service: fetch season episodes: %w", r.err)
		}
		allEpisodes = append(allEpisodes, r.episodes...)
	}

	return s.repo.UpsertTVState(ctx, userID, req.ID, allEpisodes)
}

// UpsertEpisodes adds specific episodes to the user's TV watch history.
func (s *TVService) UpsertEpisodes(ctx context.Context, userID string, req UpsertEpisodesRequest) error {
	if len(req.Episodes) == 0 {
		return fmt.Errorf("tv service: episodes must not be empty")
	}
	return s.repo.UpsertEpisodes(ctx, userID, req)
}

// GetStates returns aggregated TV tracking records for the given user.
func (s *TVService) GetStates(ctx context.Context, userID string) ([]TVStateResponse, error) {
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get states: %w", err)
	}
	return states, nil
}
