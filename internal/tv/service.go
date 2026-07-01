package tv

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

const appendToResponse = "credits,videos,watch/providers,external_ids"

// randomSeenTTL is how long a served TV id stays excluded from that user's
// future /tv/random calls. Rolling: refreshed on every call.
const randomSeenTTL = 24 * time.Hour

// randomSeenCap bounds how many ids we remember per user, dropping the oldest.
const randomSeenCap = 300

// TVService holds the business logic for the TV feature.
type TVService struct {
	repo  TVRepository
	tmdb  *tmdb.Client
	cache cache.Cache
}

// NewService constructs a TVService.
func NewService(repo TVRepository, tmdb *tmdb.Client, c cache.Cache) *TVService {
	return &TVService{repo: repo, tmdb: tmdb, cache: c}
}

func randomSeenKey(userID string) string {
	return "tv:random:seen:" + userID
}

// loadSeenIDs returns the ids previously served to this user via Random.
// Cache errors are swallowed — caching is best-effort and must never fail the request.
func (s *TVService) loadSeenIDs(ctx context.Context, userID string) []int64 {
	raw, ok, err := s.cache.Get(ctx, randomSeenKey(userID))
	if err != nil || !ok {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// saveSeenIDs merges newIDs into the user's seen-set, caps its size, and refreshes the TTL.
func (s *TVService) saveSeenIDs(ctx context.Context, userID string, existing, newIDs []int64) {
	merged := append(existing, newIDs...)
	if len(merged) > randomSeenCap {
		merged = merged[len(merged)-randomSeenCap:]
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, randomSeenKey(userID), string(raw), randomSeenTTL)
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

	// Override next_episode_to_air.air_date with the DB value which has the
	// correct timezone (set by TVMaze sync), unlike TMDB which returns date-only.
	airDates, err := s.repo.GetNextEpisodeAirDatesByIDs(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("tv service: get air dates from db: %w", err)
	}
	for i := range results {
		if airDate, ok := airDates[results[i].ID]; ok && results[i].NextEpisodeToAir != nil {
			results[i].NextEpisodeToAir.AirDate = FlexAirDate(airDate)
		}
	}

	return results, total, nil
}

// Random returns up to pageSize TV series the user hasn't tracked yet, excluding
// any series whose derived account_status (watchlist/watching/waiting_next_ep/watched)
// is in withoutStatus. Results are an even split between series with an upcoming
// episode/premiere and popular titles (e.g. pageSize=20 -> 10 upcoming + 10
// popular), topping up from the popular pool if either side comes up short.
func (s *TVService) Random(ctx context.Context, userID string, pageSize int, withoutStatus []string) ([]TVResponse, error) {
	seenIDs := s.loadSeenIDs(ctx, userID)

	upcomingQuota := pageSize / 2
	popularQuota := pageSize - upcomingQuota

	upcoming, err := s.repo.RandomIDs(ctx, userID, true, upcomingQuota, withoutStatus, seenIDs)
	if err != nil {
		return nil, fmt.Errorf("tv service: random upcoming ids: %w", err)
	}
	popular, err := s.repo.RandomIDs(ctx, userID, false, popularQuota, withoutStatus, seenIDs)
	if err != nil {
		return nil, fmt.Errorf("tv service: random popular ids: %w", err)
	}

	ids := make([]int64, 0, pageSize)
	seen := make(map[int64]struct{}, pageSize)
	for _, id := range upcoming {
		ids = append(ids, id)
		seen[id] = struct{}{}
	}
	for _, id := range popular {
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
			seen[id] = struct{}{}
		}
	}

	if len(ids) < pageSize {
		exclude := make([]int64, 0, len(seenIDs)+len(ids))
		exclude = append(exclude, seenIDs...)
		exclude = append(exclude, ids...)
		topUp, err := s.repo.RandomIDs(ctx, userID, false, pageSize-len(ids), withoutStatus, exclude)
		if err != nil {
			return nil, fmt.Errorf("tv service: random top-up ids: %w", err)
		}
		for _, id := range topUp {
			if _, ok := seen[id]; !ok {
				ids = append(ids, id)
				seen[id] = struct{}{}
			}
		}
	}

	if len(ids) == 0 {
		return []TVResponse{}, nil
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
			return nil, fmt.Errorf("tv service: enrich random from tmdb: %w", err)
		}
	}

	airDates, err := s.repo.GetNextEpisodeAirDatesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("tv service: get air dates from db: %w", err)
	}
	for i := range results {
		if airDate, ok := airDates[results[i].ID]; ok && results[i].NextEpisodeToAir != nil {
			results[i].NextEpisodeToAir.AirDate = FlexAirDate(airDate)
		}
	}

	s.saveSeenIDs(ctx, userID, seenIDs, ids)

	return results, nil
}

// searchResult is a minimal TMDB search result used only to collect IDs.
type searchResult struct {
	ID int64 `json:"id"`
}

// Search queries TMDB for TV series, fetches full detail for each result, then merges
// with cached DB records (DB fields take precedence for vote_average/vote_count).
func (s *TVService) Search(ctx context.Context, query string, page int) (*response.Page[TVResponse], error) {
	var tmdbResult tmdb.SearchPage[searchResult]
	if err := s.tmdb.SearchTV(ctx, query, page, &tmdbResult); err != nil {
		return nil, fmt.Errorf("tv service: search tmdb: %w", err)
	}

	if len(tmdbResult.Results) == 0 {
		return &response.Page[TVResponse]{
			Page:         tmdbResult.Page,
			TotalPages:   tmdbResult.TotalPages,
			TotalResults: tmdbResult.TotalResults,
			Results:      []TVResponse{},
		}, nil
	}

	ids := make([]int64, len(tmdbResult.Results))
	for i, r := range tmdbResult.Results {
		ids[i] = r.ID
	}

	details := make([]TVResponse, len(ids))
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
			details[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("tv service: search enrich from tmdb: %w", err)
		}
	}

	cached, err := s.repo.FindByTMDBIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("tv service: fetch cached tv: %w", err)
	}

	for i, detail := range details {
		if db, ok := cached[detail.ID]; ok {
			if db.VoteAverage != nil {
				details[i].VoteAverage = *db.VoteAverage
			}
			if db.VoteCount != nil {
				details[i].VoteCount = *db.VoteCount
			}
		}
	}

	return &response.Page[TVResponse]{
		Page:         tmdbResult.Page,
		TotalPages:   tmdbResult.TotalPages,
		TotalResults: tmdbResult.TotalResults,
		Results:      details,
	}, nil
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
// For episodes where episode_id is absent, it is resolved from TMDB concurrently.
func (s *TVService) UpsertEpisodes(ctx context.Context, userID string, req UpsertEpisodesRequest) error {
	if len(req.Episodes) == 0 {
		return fmt.Errorf("tv service: episodes must not be empty")
	}

	// Resolve missing episode IDs from TMDB in parallel.
	type idResult struct {
		id  int64
		err error
	}
	results := make([]idResult, len(req.Episodes))

	var wg sync.WaitGroup
	for i, ep := range req.Episodes {
		if ep.EpisodeID != nil {
			continue
		}
		wg.Add(1)
		go func(idx int, ep EpisodeInput) {
			defer wg.Done()
			var detail struct {
				ID int64 `json:"id"`
			}
			if err := s.tmdb.GetTVEpisode(ctx, req.ID, ep.SeasonNumber, ep.EpisodeNumber, &detail); err != nil {
				results[idx].err = err
				return
			}
			results[idx].id = detail.ID
		}(i, ep)
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			ep := req.Episodes[i]
			return fmt.Errorf("tv service: fetch episode id s%de%d: %w", ep.SeasonNumber, ep.EpisodeNumber, r.err)
		}
		if req.Episodes[i].EpisodeID == nil && r.id != 0 {
			id := r.id
			req.Episodes[i].EpisodeID = &id
		}
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
