package movie

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

const appendToResponse = "credits,videos,watch/providers,release_dates,external_ids,keywords"
const detailAppendToResponse = appendToResponse + ",recommendations"

// seenTTL is how long a served movie id stays excluded from that user's
// future /movie/random or /movie/recommendations calls. Rolling: refreshed
// on every call.
const seenTTL = 24 * time.Hour

// seenCap bounds how many ids we remember per user/feature, dropping the oldest.
const seenCap = 300

// randomMaxPages caps how many trending pages Random will fetch while trying
// to fill total.
const randomMaxPages = 5

// reelsMaxPages widens the random source page range for the trailer feed so
// its seen cache does not cycle through the same small popular pool quickly.
const reelsMaxPages = 25

// randomTimeWindow is the trending time window Random samples from.
const randomTimeWindow = "day"

// recommendationsPageSize is the fixed page size for /movie/recommendations,
// matching Discover's default page size.
const recommendationsPageSize = 20

// recommendationCandidateLimit bounds how many candidate documents are
// fetched from the local cache and scored in Go per request.
const recommendationCandidateLimit = 300

// MovieAffinity holds a user's recommendation-profile scores (-100..100) for
// the entity buckets relevant to ranking movie candidates, keyed by TMDB
// entity id, plus the profile's watched-titles exclusion list. Defined here
// (rather than imported from the recommendation package) to avoid a cyclic
// dependency — recommendation already imports movie for its
// repository/model types.
type MovieAffinity struct {
	Genres              map[int64]int
	Keywords            map[int64]int
	Actors              map[int64]int
	Directors           map[int64]int
	Collections         map[int64]int
	ProductionCompanies map[int64]int
	WatchedIDs          []int64
}

// RecommendationUpdater is the narrow interface MovieService uses to trigger
// a recommendation-profile update after a state change and to read back a
// user's affinity scores for the /movie/recommendations endpoint, without
// importing the recommendation package's concrete type. Errors from
// ApplyMovieStateChange are handled internally by the implementation — it
// never fails the caller's request.
type RecommendationUpdater interface {
	ApplyMovieStateChange(ctx context.Context, userID string, movieID int64)
	GetMovieAffinity(ctx context.Context, userID string) (MovieAffinity, error)
}

// MovieService holds the business logic for the movie feature.
type MovieService struct {
	repo           MovieRepository
	tmdb           *tmdb.Client
	cache          cache.Cache
	recommendation RecommendationUpdater
}

// NewService constructs a MovieService.
func NewService(repo MovieRepository, tmdb *tmdb.Client, c cache.Cache, rec RecommendationUpdater) *MovieService {
	return &MovieService{repo: repo, tmdb: tmdb, cache: c, recommendation: rec}
}

func randomSeenKey(userID string) string {
	return "movie:random:seen:" + userID
}

func reelsSeenKey(userID string) string {
	return "movie:reels:seen:" + userID
}

// recommendationSeenKey namespaces the seen-id cache for /movie/recommendations,
// separate from Random's, so serving a title via one endpoint doesn't
// suppress it on the other.
func recommendationSeenKey(userID string) string {
	return "movie:recommendations:seen:" + userID
}

func randomRecommendationSeenKey(userID string) string {
	return "movie:recommendations:random:seen:" + userID
}

// loadSeenIDs returns the ids previously served to this user under the given
// cache key. Cache errors are swallowed — caching is best-effort and must
// never fail the request.
func (s *MovieService) loadSeenIDs(ctx context.Context, key string) []int64 {
	raw, ok, err := s.cache.Get(ctx, key)
	if err != nil || !ok {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// saveSeenIDs merges newIDs into the seen-set under the given cache key,
// caps its size, and refreshes the TTL.
func (s *MovieService) saveSeenIDs(ctx context.Context, key string, existing, newIDs []int64) {
	merged := append(existing, newIDs...)
	if len(merged) > seenCap {
		merged = merged[len(merged)-seenCap:]
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, key, string(raw), seenTTL)
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

	results, err := s.enrichMovieIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetByIDs returns the same enriched response shape as Discover for a known
// set of movie IDs.
func (s *MovieService) GetByIDs(ctx context.Context, ids []int64) ([]MovieResponse, error) {
	if len(ids) == 0 {
		return []MovieResponse{}, nil
	}
	return s.enrichMovieIDs(ctx, ids)
}

// enrichMovieIDs fetches full TMDB detail only for movies already present in
// the local catalog, then overlays cached DB fields. The catalog is the gate
// for every list returned by this service; TMDB only enriches those records.
func (s *MovieService) enrichMovieIDs(ctx context.Context, ids []int64) ([]MovieResponse, error) {
	// Load cached DB records so they can override TMDB detail. Read-only
	// during the fan-out below.
	cached, err := s.repo.FindByTMDBIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("movie service: fetch cached movies: %w", err)
	}
	knownIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := cached[id]; ok {
			knownIDs = append(knownIDs, id)
		}
	}
	if len(knownIDs) == 0 {
		return []MovieResponse{}, nil
	}

	results := make([]MovieResponse, len(knownIDs))
	errs := make([]error, len(knownIDs))

	var wg sync.WaitGroup
	for i, id := range knownIDs {
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
			if db, ok := cached[movieID]; ok {
				overlayDBFields(&detail, db)
			}
			results[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("movie service: enrich from tmdb: %w", err)
		}
	}

	return results, nil
}

// Recommendations returns a page of movies ranked by the authenticated
// user's actual computed recommendation-profile affinity score (see
// rankByAffinity), excluding titles the profile already counts as watched
// and titles already served to this user by a previous call to this
// endpoint (tracked via a rolling seen-id cache, see recommendationSeenKey).
// Returns an empty result (not an error) when no recommendation service is
// wired or the user has no positive profile signal yet — callers should
// treat that as "not enough data", the same as a cold-start user; this
// endpoint intentionally does not fall back to trending/popular titles.
func (s *MovieService) Recommendations(ctx context.Context, userID string, page int) ([]MovieResponse, int, error) {
	if s.recommendation == nil {
		return []MovieResponse{}, 0, nil
	}

	aff, err := s.recommendation.GetMovieAffinity(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("movie service: get movie affinity: %w", err)
	}

	criteria := RecommendationCriteria{
		GenreIDs:      positiveScoreIDs(aff.Genres),
		KeywordIDs:    positiveScoreIDs(aff.Keywords),
		CastIDs:       positiveScoreIDs(aff.Actors),
		DirectorIDs:   positiveScoreIDs(aff.Directors),
		CollectionIDs: positiveScoreIDs(aff.Collections),
		CompanyIDs:    positiveScoreIDs(aff.ProductionCompanies),
	}
	if criteria.empty() {
		return []MovieResponse{}, 0, nil
	}

	candidates, err := s.repo.FindRecommendationCandidates(ctx, criteria, aff.WatchedIDs, recommendationCandidateLimit)
	if err != nil {
		return nil, 0, fmt.Errorf("movie service: find recommendation candidates: %w", err)
	}

	seenIDs := s.loadSeenIDs(ctx, recommendationSeenKey(userID))
	seenSet := make(map[int64]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		seenSet[id] = struct{}{}
	}

	scored := make([]Movie, 0, len(candidates))
	for _, m := range candidates {
		if _, ok := seenSet[m.MovieID]; ok {
			continue
		}
		if isAcceptableMovieDoc(m) {
			scored = append(scored, m)
		}
	}
	scored = rankByAffinity(scored, aff)

	total := len(scored)
	start := (page - 1) * recommendationsPageSize
	if start >= total {
		return []MovieResponse{}, total, nil
	}
	end := start + recommendationsPageSize
	if end > total {
		end = total
	}

	pageIDs := make([]int64, end-start)
	for i, m := range scored[start:end] {
		pageIDs[i] = m.MovieID
	}

	results, err := s.enrichMovieIDs(ctx, pageIDs)
	if err != nil {
		return nil, 0, err
	}
	s.saveSeenIDs(ctx, recommendationSeenKey(userID), seenIDs, pageIDs)
	return results, total, nil
}

// RandomRecommendations returns a random personalized selection. It uses the
// same affinity criteria as Recommendations, but samples from eligible titles
// and remembers served ids so consecutive calls do not repeat picks.
func (s *MovieService) RandomRecommendations(ctx context.Context, userID string, total int) ([]MovieResponse, error) {
	return s.randomRecommendations(ctx, userID, total, true)
}

func (s *MovieService) randomRecommendations(ctx context.Context, userID string, total int, resetSeenWhenExhausted bool) ([]MovieResponse, error) {
	if s.recommendation == nil {
		return []MovieResponse{}, nil
	}

	aff, err := s.recommendation.GetMovieAffinity(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("movie service: get movie affinity: %w", err)
	}
	criteria := RecommendationCriteria{
		GenreIDs:      positiveScoreIDs(aff.Genres),
		KeywordIDs:    positiveScoreIDs(aff.Keywords),
		CastIDs:       positiveScoreIDs(aff.Actors),
		DirectorIDs:   positiveScoreIDs(aff.Directors),
		CollectionIDs: positiveScoreIDs(aff.Collections),
		CompanyIDs:    positiveScoreIDs(aff.ProductionCompanies),
	}
	if criteria.empty() {
		return []MovieResponse{}, nil
	}

	candidates, err := s.repo.FindRecommendationCandidates(ctx, criteria, aff.WatchedIDs, recommendationCandidateLimit)
	if err != nil {
		return nil, fmt.Errorf("movie service: find random recommendation candidates: %w", err)
	}
	seenIDs := s.loadSeenIDs(ctx, randomRecommendationSeenKey(userID))
	seenSet := make(map[int64]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		seenSet[id] = struct{}{}
	}

	eligible := make([]Movie, 0, len(candidates))
	for _, candidate := range candidates {
		if _, seen := seenSet[candidate.MovieID]; seen || !isAcceptableMovieDoc(candidate) {
			continue
		}
		eligible = append(eligible, candidate)
	}
	rand.Shuffle(len(eligible), func(i, j int) { eligible[i], eligible[j] = eligible[j], eligible[i] })

	personalizedTotal := min(len(eligible), (total*7+9)/10)
	selectedIDs := make(map[int64]struct{}, total)
	ids := make([]int64, 0, total)
	for _, candidate := range eligible[:personalizedTotal] {
		selectedIDs[candidate.MovieID] = struct{}{}
		ids = append(ids, candidate.MovieID)
	}

	watchedSet := make(map[int64]struct{}, len(aff.WatchedIDs))
	for _, id := range aff.WatchedIDs {
		watchedSet[id] = struct{}{}
	}
	trendingIDs := make([]int64, 0, total-personalizedTotal)
	for page := 1; page <= randomMaxPages && len(ids)+len(trendingIDs) < total; page++ {
		trendingPage, err := s.Trending(ctx, randomTimeWindow, page)
		if err != nil {
			return nil, fmt.Errorf("movie service: random recommendation trending page %d: %w", page, err)
		}
		for _, movie := range trendingPage.Results {
			if _, watched := watchedSet[movie.ID]; watched {
				continue
			}
			if _, seen := seenSet[movie.ID]; seen {
				continue
			}
			if _, selected := selectedIDs[movie.ID]; selected {
				continue
			}
			selectedIDs[movie.ID] = struct{}{}
			trendingIDs = append(trendingIDs, movie.ID)
			if len(ids)+len(trendingIDs) == total {
				break
			}
		}
	}
	rand.Shuffle(len(trendingIDs), func(i, j int) { trendingIDs[i], trendingIDs[j] = trendingIDs[j], trendingIDs[i] })
	ids = append(ids, trendingIDs...)

	if len(ids) < total {
		discoverIDs, err := s.discoverRecommendationIDs(ctx, positiveScoreIDs(aff.Genres), watchedSet, seenSet, selectedIDs, total-len(ids))
		if err != nil {
			return nil, err
		}
		ids = append(ids, discoverIDs...)
	}

	for _, candidate := range eligible[personalizedTotal:] {
		if len(ids) == total {
			break
		}
		if _, selected := selectedIDs[candidate.MovieID]; selected {
			continue
		}
		selectedIDs[candidate.MovieID] = struct{}{}
		ids = append(ids, candidate.MovieID)
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	if len(ids) == 0 && resetSeenWhenExhausted && len(seenIDs) > 0 {
		_ = s.cache.Delete(ctx, randomRecommendationSeenKey(userID))
		return s.randomRecommendations(ctx, userID, total, false)
	}
	results, err := s.enrichMovieIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	s.saveSeenIDs(ctx, randomRecommendationSeenKey(userID), seenIDs, ids)
	return results, nil
}

// discoverRecommendationIDs broadens a depleted personalized pool with
// unwatched, unseen titles from TMDB Discover, using one of the user's liked
// genres to retain relevance.
func (s *MovieService) discoverRecommendationIDs(ctx context.Context, genreIDs []int64, watchedSet, seenSet, selectedIDs map[int64]struct{}, total int) ([]int64, error) {
	if total == 0 {
		return nil, nil
	}

	var genreID int64
	if len(genreIDs) > 0 {
		genreID = genreIDs[rand.Intn(len(genreIDs))]
	}
	ids := make([]int64, 0, total)
	for page := 1; page <= randomMaxPages && len(ids) < total; page++ {
		discoverPage, err := s.tmdb.Discover(ctx, "movie", genreID, page)
		if err != nil {
			return nil, fmt.Errorf("movie service: discover recommendation page %d: %w", page, err)
		}
		candidateIDs := make([]int64, 0, len(discoverPage.Results))
		for _, movie := range discoverPage.Results {
			candidateIDs = append(candidateIDs, movie.ID)
		}
		cached, err := s.repo.FindByTMDBIDs(ctx, candidateIDs)
		if err != nil {
			return nil, fmt.Errorf("movie service: discover recommendation cached ids: %w", err)
		}
		for _, movie := range discoverPage.Results {
			if _, known := cached[movie.ID]; !known {
				continue
			}
			if _, watched := watchedSet[movie.ID]; watched {
				continue
			}
			if _, seen := seenSet[movie.ID]; seen {
				continue
			}
			if _, selected := selectedIDs[movie.ID]; selected {
				continue
			}
			selectedIDs[movie.ID] = struct{}{}
			ids = append(ids, movie.ID)
			if len(ids) == total {
				break
			}
		}
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids, nil
}

// positiveScoreIDs returns the entity ids with a positive score — only
// entities the user actually likes are worth matching candidates against.
func positiveScoreIDs(scores map[int64]int) []int64 {
	ids := make([]int64, 0, len(scores))
	for id, score := range scores {
		if score > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// Random returns up to total movies the user hasn't tracked yet (per withoutStatus),
// sampled by shuffling TMDB's trending pool (day window, already filtered through
// isAcceptableMovie via Trending). Trending is paged (up to randomMaxPages) until
// enough untracked, unseen candidates are gathered or trending is exhausted. Ids
// served recently (see the Redis "seen" cache) are excluded so consecutive calls
// don't repeat the same titles.
func (s *MovieService) Random(ctx context.Context, userID string, total int, withoutStatus []string) ([]MovieResponse, error) {
	seenIDs := s.loadSeenIDs(ctx, randomSeenKey(userID))
	seenSet := make(map[int64]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		seenSet[id] = struct{}{}
	}

	excludeSet, err := s.trackedExcludeIDs(ctx, userID, withoutStatus)
	if err != nil {
		return nil, err
	}

	pool := make([]MovieResponse, 0, total)
	poolSet := make(map[int64]struct{}, total)

	for page := 1; page <= randomMaxPages && len(pool) < total; page++ {
		trendingPage, err := s.Trending(ctx, randomTimeWindow, page)
		if err != nil {
			return nil, fmt.Errorf("movie service: random trending page %d: %w", page, err)
		}
		if len(trendingPage.Results) == 0 {
			break
		}
		for _, m := range trendingPage.Results {
			if _, ok := poolSet[m.ID]; ok {
				continue
			}
			if _, ok := excludeSet[m.ID]; ok {
				continue
			}
			if _, ok := seenSet[m.ID]; ok {
				continue
			}
			poolSet[m.ID] = struct{}{}
			pool = append(pool, m)
		}
	}

	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > total {
		pool = pool[:total]
	}

	ids := make([]int64, len(pool))
	for i, m := range pool {
		ids[i] = m.ID
	}
	s.saveSeenIDs(ctx, randomSeenKey(userID), seenIDs, ids)

	return pool, nil
}

// Reels creates a small, varied trailer feed without making the client stitch
// together multiple sources. About half comes from the user's current
// watchlist; the rest is split between random pages of TMDB trending and
// popular. Watched and watchlisted titles are excluded from the external
// sources, while a separate per-user seen cache prevents consecutive feeds
// from repeating titles.
func (s *MovieService) Reels(ctx context.Context, userID string, total int) ([]MovieResponse, error) {
	return s.reels(ctx, userID, total, true)
}

func (s *MovieService) reels(ctx context.Context, userID string, total int, resetSeenWhenExhausted bool) ([]MovieResponse, error) {
	seenIDs := s.loadSeenIDs(ctx, reelsSeenKey(userID))
	seenSet := make(map[int64]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		seenSet[id] = struct{}{}
	}

	states, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("movie service: reels states: %w", err)
	}
	trackedSet := make(map[int64]struct{}, len(states))
	watchlistIDs := make([]int64, 0, len(states))
	for _, state := range states {
		trackedSet[state.MovieID] = struct{}{}
		if state.WatchedAt == nil {
			watchlistIDs = append(watchlistIDs, state.MovieID)
		}
	}
	rand.Shuffle(len(watchlistIDs), func(i, j int) {
		watchlistIDs[i], watchlistIDs[j] = watchlistIDs[j], watchlistIDs[i]
	})

	selectedSet := make(map[int64]struct{}, total)
	ids := make([]int64, 0, total)
	addIDs := func(candidates []int64, limit int, excludeTracked bool) {
		for _, id := range candidates {
			if len(ids) == total || (limit > 0 && len(ids) >= limit) {
				return
			}
			if _, seen := seenSet[id]; seen {
				continue
			}
			if _, selected := selectedSet[id]; selected {
				continue
			}
			if excludeTracked {
				if _, tracked := trackedSet[id]; tracked {
					continue
				}
			}
			selectedSet[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	trendingPage, err := s.tmdb.GetTrending(ctx, "movie", randomTimeWindow, rand.Intn(reelsMaxPages)+1)
	if err != nil {
		return nil, fmt.Errorf("movie service: reels trending: %w", err)
	}
	trendingIDs := make([]int64, 0, len(trendingPage.Results))
	for _, movie := range trendingPage.Results {
		trendingIDs = append(trendingIDs, movie.ID)
	}
	rand.Shuffle(len(trendingIDs), func(i, j int) { trendingIDs[i], trendingIDs[j] = trendingIDs[j], trendingIDs[i] })

	popularPage, err := s.tmdb.Discover(ctx, "movie", 0, rand.Intn(reelsMaxPages)+1)
	if err != nil {
		return nil, fmt.Errorf("movie service: reels popular: %w", err)
	}
	popularIDs := make([]int64, 0, len(popularPage.Results))
	for _, movie := range popularPage.Results {
		popularIDs = append(popularIDs, movie.ID)
	}
	allCandidateIDs := make([]int64, 0, len(watchlistIDs)+len(trendingIDs)+len(popularIDs))
	allCandidateIDs = append(allCandidateIDs, watchlistIDs...)
	allCandidateIDs = append(allCandidateIDs, trendingIDs...)
	allCandidateIDs = append(allCandidateIDs, popularIDs...)
	cached, err := s.repo.FindByTMDBIDs(ctx, allCandidateIDs)
	if err != nil {
		return nil, fmt.Errorf("movie service: reels cached ids: %w", err)
	}
	filterKnown := func(candidates []int64) []int64 {
		filtered := make([]int64, 0, len(candidates))
		for _, id := range candidates {
			if _, known := cached[id]; known {
				filtered = append(filtered, id)
			}
		}
		return filtered
	}
	watchlistIDs = filterKnown(watchlistIDs)
	trendingIDs = filterKnown(trendingIDs)
	popularIDs = filterKnown(popularIDs)

	watchlistTarget := (total + 1) / 2
	addIDs(watchlistIDs, watchlistTarget, false)
	trendingTarget := len(ids) + (total-len(ids)+1)/2
	addIDs(trendingIDs, trendingTarget, true)
	rand.Shuffle(len(popularIDs), func(i, j int) { popularIDs[i], popularIDs[j] = popularIDs[j], popularIDs[i] })
	addIDs(popularIDs, total, true)

	// One source can be sparse (for example a short watchlist), so use the
	// remaining candidates as backfill before returning a partial feed.
	addIDs(watchlistIDs, total, false)
	addIDs(trendingIDs, total, true)
	addIDs(popularIDs, total, true)

	if len(ids) == 0 && resetSeenWhenExhausted && len(seenIDs) > 0 {
		_ = s.cache.Delete(ctx, reelsSeenKey(userID))
		return s.reels(ctx, userID, total, false)
	}

	results, err := s.enrichMovieIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	filtered := make([]MovieResponse, 0, len(results))
	for _, movie := range results {
		if isAcceptableMovie(movie) {
			filtered = append(filtered, movie)
		}
	}
	rand.Shuffle(len(filtered), func(i, j int) { filtered[i], filtered[j] = filtered[j], filtered[i] })

	servedIDs := make([]int64, len(filtered))
	for i, movie := range filtered {
		servedIDs[i] = movie.ID
	}
	s.saveSeenIDs(ctx, reelsSeenKey(userID), seenIDs, servedIDs)
	return filtered, nil
}

// trackedExcludeIDs returns the set of movie ids the user has tracked with an
// account_status in withoutStatus (nil/empty withoutStatus excludes nothing).
func (s *MovieService) trackedExcludeIDs(ctx context.Context, userID string, withoutStatus []string) (map[int64]struct{}, error) {
	excludeSet := make(map[int64]struct{})
	if len(withoutStatus) == 0 {
		return excludeSet, nil
	}
	statuses := make(map[string]struct{}, len(withoutStatus))
	for _, s := range withoutStatus {
		statuses[s] = struct{}{}
	}
	states, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("movie service: tracked exclude ids: %w", err)
	}
	for _, st := range states {
		status := accountStatus(st.WatchedAt)
		if status != nil {
			if _, ok := statuses[*status]; ok {
				excludeSet[st.MovieID] = struct{}{}
			}
		}
	}
	return excludeSet, nil
}

// Search finds movies in the local catalog by title, then enriches the matched
// titles with their full TMDB detail payload.
func (s *MovieService) Search(ctx context.Context, query string, page int) (*response.Page[MovieResponse], error) {
	ids, total, err := s.repo.SearchIDs(ctx, query, page)
	if err != nil {
		return nil, fmt.Errorf("movie service: search local catalog: %w", err)
	}
	if len(ids) == 0 {
		return &response.Page[MovieResponse]{Page: page, TotalPages: max(1, (total+19)/20), TotalResults: total, Results: []MovieResponse{}}, nil
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
				errs[idx] = err
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
			overlayDBFields(&details[i], db)
		}
	}
	return &response.Page[MovieResponse]{Page: page, TotalPages: max(1, (total+19)/20), TotalResults: total, Results: details}, nil
}

// Trending returns TMDB's trending movies for the given time window, enriched with full
// detail and filtered through isAcceptableMovie. Cached DB records win over TMDB detail
// for the fields the DB is the source of truth for (see overlayDBFields).
func (s *MovieService) Trending(ctx context.Context, timeWindow string, page int) (*response.Page[MovieResponse], error) {
	trendingPage, err := s.tmdb.GetTrending(ctx, "movie", timeWindow, page)
	if err != nil {
		return nil, fmt.Errorf("movie service: trending tmdb: %w", err)
	}

	if len(trendingPage.Results) == 0 {
		return &response.Page[MovieResponse]{
			Page:         trendingPage.Page,
			TotalPages:   trendingPage.TotalPages,
			TotalResults: trendingPage.TotalResults,
			Results:      []MovieResponse{},
		}, nil
	}

	ids := make([]int64, len(trendingPage.Results))
	for i, r := range trendingPage.Results {
		ids[i] = r.ID
	}
	details, err := s.enrichMovieIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("movie service: trending enrich from tmdb: %w", err)
	}

	results := make([]MovieResponse, 0, len(details))
	for _, detail := range details {
		if !isAcceptableMovie(detail) {
			continue
		}
		results = append(results, detail)
	}

	return &response.Page[MovieResponse]{
		Page:         trendingPage.Page,
		TotalPages:   trendingPage.TotalPages,
		TotalResults: trendingPage.TotalResults,
		Results:      results,
	}, nil
}

// GetByID returns detail for a single movie when it exists in the local catalog.
// When the movie is cached in the DB,
// the DB record is the source of truth for the fields it stores (popularity,
// votes, release dates, status, runtime, title, poster, language) and TMDB
// supplies the rest (overview, cast, videos, genre names, watch providers).
// Movies absent from the DB are not exposed. Returns tmdb.ErrNotFound if the
// catalog or TMDB has no such movie.
func (s *MovieService) GetByID(ctx context.Context, id int64) (*MovieResponse, error) {
	cached, err := s.repo.FindByTMDBIDs(ctx, []int64{id})
	if err != nil {
		return nil, fmt.Errorf("movie service: fetch cached movie: %w", err)
	}
	db, ok := cached[id]
	if !ok {
		return nil, fmt.Errorf("movie service: get by id %d: %w", id, tmdb.ErrNotFound)
	}

	var detail MovieResponse
	if err := s.tmdb.GetMovieDetail(ctx, id, detailAppendToResponse, &detail); err != nil {
		return nil, fmt.Errorf("movie service: get by id %d: %w", id, err)
	}
	detail.MediaType = "movie"
	if detail.ImdbID == "" && detail.ExternalIDs != nil {
		detail.ImdbID = detail.ExternalIDs.ImdbID
	}
	detail.ReleaseDateTH = extractReleaseDateTH(detail)

	overlayDBFields(&detail, db)
	return &detail, nil
}

// UpsertState creates or updates the user's movie tracking record.
func (s *MovieService) UpsertState(ctx context.Context, userID string, req UpsertStateRequest) error {
	if req.Status != "watched" && req.Status != "watchlist" {
		return fmt.Errorf("movie service: invalid status %q", req.Status)
	}
	if req.Rating != nil && (*req.Rating < 0 || *req.Rating > 10) {
		return fmt.Errorf("movie service: invalid rating %v", *req.Rating)
	}
	if err := s.repo.UpsertState(ctx, userID, req); err != nil {
		return err
	}
	if s.recommendation != nil {
		s.recommendation.ApplyMovieStateChange(ctx, userID, req.ID)
	}
	return nil
}

func accountStatus(watchedAt *time.Time) *string {
	if watchedAt != nil {
		s := "watched"
		return &s
	}
	s := "watchlist"
	return &s
}
