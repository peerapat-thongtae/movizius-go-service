package movie

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

const appendToResponse = "casts,videos,watch/providers,release_dates,external_ids"

// randomSeenTTL is how long a served movie id stays excluded from that user's
// future /movie/random calls. Rolling: refreshed on every call.
const randomSeenTTL = 24 * time.Hour

// randomSeenCap bounds how many ids we remember per user, dropping the oldest.
const randomSeenCap = 300

// MovieService holds the business logic for the movie feature.
type MovieService struct {
	repo  MovieRepository
	tmdb  *tmdb.Client
	cache cache.Cache
}

// NewService constructs a MovieService.
func NewService(repo MovieRepository, tmdb *tmdb.Client, c cache.Cache) *MovieService {
	return &MovieService{repo: repo, tmdb: tmdb, cache: c}
}

func randomSeenKey(userID string) string {
	return "movie:random:seen:" + userID
}

// loadSeenIDs returns the ids previously served to this user via Random.
// Cache errors are swallowed — caching is best-effort and must never fail the request.
func (s *MovieService) loadSeenIDs(ctx context.Context, userID string) []int64 {
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
func (s *MovieService) saveSeenIDs(ctx context.Context, userID string, existing, newIDs []int64) {
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
	if err != nil {
		return nil, 0, fmt.Errorf("movie service: discover ids: %w", err)
	}
	if len(ids) == 0 {
		return []MovieResponse{}, total, nil
	}

	results := make([]MovieResponse, len(ids))
	errs := make([]error, len(ids))

	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, movieID int64) {
			defer wg.Done()
			var detail MovieResponse
			if err := s.tmdb.GetMovieDetail(ctx, movieID, appendToResponse, &detail); err != nil {
				errs[idx] = fmt.Errorf("tmdb detail for id %d: %w", movieID, err)
				return
			}
			detail.MediaType = "movie"
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
			}
			detail.ReleaseDateTH = extractReleaseDateTH(detail)
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

// Random returns up to pageSize movies the user hasn't tracked yet, excluding any
// movie whose account_status (derived from movie_user) matches withoutStatus.
// Results are an even split between upcoming releases and popular titles (e.g.
// pageSize=20 -> 10 upcoming + 10 popular), topping up from the popular pool
// if either side comes up short.
func (s *MovieService) Random(ctx context.Context, userID string, pageSize int, withoutStatus []string) ([]MovieResponse, error) {
	seenIDs := s.loadSeenIDs(ctx, userID)

	upcomingQuota := pageSize / 2
	popularQuota := pageSize - upcomingQuota

	upcoming, err := s.repo.RandomIDs(ctx, userID, true, upcomingQuota, withoutStatus, seenIDs)
	if err != nil {
		return nil, fmt.Errorf("movie service: random upcoming ids: %w", err)
	}
	popular, err := s.repo.RandomIDs(ctx, userID, false, popularQuota, withoutStatus, seenIDs)
	if err != nil {
		return nil, fmt.Errorf("movie service: random popular ids: %w", err)
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
			return nil, fmt.Errorf("movie service: random top-up ids: %w", err)
		}
		for _, id := range topUp {
			if _, ok := seen[id]; !ok {
				ids = append(ids, id)
				seen[id] = struct{}{}
			}
		}
	}

	if len(ids) == 0 {
		return []MovieResponse{}, nil
	}

	results := make([]MovieResponse, len(ids))
	errs := make([]error, len(ids))

	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, movieID int64) {
			defer wg.Done()
			var detail MovieResponse
			if err := s.tmdb.GetMovieDetail(ctx, movieID, appendToResponse, &detail); err != nil {
				errs[idx] = fmt.Errorf("tmdb detail for id %d: %w", movieID, err)
				return
			}
			detail.MediaType = "movie"
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
			}
			detail.ReleaseDateTH = extractReleaseDateTH(detail)
			results[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("movie service: enrich random from tmdb: %w", err)
		}
	}

	s.saveSeenIDs(ctx, userID, seenIDs, ids)

	return results, nil
}

// searchResult is a minimal TMDB search result used only to collect IDs.
type searchResult struct {
	ID int64 `json:"id"`
}

// Search queries TMDB for movies, fetches full detail for each result, then merges
// with cached DB records (DB fields take precedence for vote_average/vote_count).
func (s *MovieService) Search(ctx context.Context, query string, page int) (*response.Page[MovieResponse], error) {
	var tmdbResult tmdb.SearchPage[searchResult]
	if err := s.tmdb.SearchMovie(ctx, query, page, &tmdbResult); err != nil {
		return nil, fmt.Errorf("movie service: search tmdb: %w", err)
	}

	if len(tmdbResult.Results) == 0 {
		return &response.Page[MovieResponse]{
			Page:         tmdbResult.Page,
			TotalPages:   tmdbResult.TotalPages,
			TotalResults: tmdbResult.TotalResults,
			Results:      []MovieResponse{},
		}, nil
	}

	ids := make([]int64, len(tmdbResult.Results))
	for i, r := range tmdbResult.Results {
		ids[i] = r.ID
	}

	details := make([]MovieResponse, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, movieID int64) {
			defer wg.Done()
			var detail MovieResponse
			if err := s.tmdb.GetMovieDetail(ctx, movieID, appendToResponse, &detail); err != nil {
				errs[idx] = fmt.Errorf("tmdb detail for id %d: %w", movieID, err)
				return
			}
			detail.MediaType = "movie"
			if detail.ImdbID == "" && detail.ExternalIDs != nil {
				detail.ImdbID = detail.ExternalIDs.ImdbID
			}
			detail.ReleaseDateTH = extractReleaseDateTH(detail)
			details[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("movie service: search enrich from tmdb: %w", err)
		}
	}

	cached, err := s.repo.FindByTMDBIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("movie service: fetch cached movies: %w", err)
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

	return &response.Page[MovieResponse]{
		Page:         tmdbResult.Page,
		TotalPages:   tmdbResult.TotalPages,
		TotalResults: tmdbResult.TotalResults,
		Results:      details,
	}, nil
}

// GetByID returns full TMDB detail for a single movie, with vote_average/vote_count
// overlaid from the DB cache (IMDB-sourced, see CLAUDE.md). Returns tmdb.ErrNotFound
// if TMDB has no such movie.
func (s *MovieService) GetByID(ctx context.Context, id int64) (*MovieResponse, error) {
	var detail MovieResponse
	if err := s.tmdb.GetMovieDetail(ctx, id, appendToResponse, &detail); err != nil {
		return nil, fmt.Errorf("movie service: get by id %d: %w", id, err)
	}
	detail.MediaType = "movie"
	if detail.ImdbID == "" && detail.ExternalIDs != nil {
		detail.ImdbID = detail.ExternalIDs.ImdbID
	}
	detail.ReleaseDateTH = extractReleaseDateTH(detail)

	cached, err := s.repo.FindByTMDBIDs(ctx, []int64{id})
	if err != nil {
		return nil, fmt.Errorf("movie service: fetch cached movie: %w", err)
	}
	if db, ok := cached[id]; ok {
		if db.VoteAverage != nil {
			detail.VoteAverage = *db.VoteAverage
		}
		if db.VoteCount != nil {
			detail.VoteCount = *db.VoteCount
		}
	}
	return &detail, nil
}

// UpsertState creates or updates the user's movie tracking record.
func (s *MovieService) UpsertState(ctx context.Context, userID string, req UpsertStateRequest) error {
	if req.Status != "watched" && req.Status != "watchlist" {
		return fmt.Errorf("movie service: invalid status %q", req.Status)
	}
	return s.repo.UpsertState(ctx, userID, req)
}

func accountStatus(watchedAt *time.Time) *string {
	if watchedAt != nil {
		s := "watched"
		return &s
	}
	s := "watchlist"
	return &s
}
