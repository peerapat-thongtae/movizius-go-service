package tv

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

const appendToResponse = "credits,videos,watch/providers,external_ids,keywords"
const detailAppendToResponse = appendToResponse + ",recommendations"

// randomSeenTTL is how long a served TV id stays excluded from that user's
// future /tv/random calls. Rolling: refreshed on every call.
const randomSeenTTL = 24 * time.Hour

// randomSeenCap bounds how many ids we remember per user, dropping the oldest.
const randomSeenCap = 300

// randomMaxPages caps how many trending pages Random will fetch while trying
// to fill total.
const randomMaxPages = 5

// randomTimeWindow is the trending time window Random samples from.
const randomTimeWindow = "day"

const recommendationsPageSize = 20
const recommendationCandidateLimit = 300

// Hero is a fixed curated rail rather than a paginated browsing surface.
const heroResultLimit = 20
const heroMaxPages = 25
const waitingNextEpisodeResultLimit = 20
const topRatedResultLimit = 20

type TVAffinity struct {
	Genres              map[int64]int
	Keywords            map[int64]int
	Actors              map[int64]int
	Creators            map[int64]int
	ProductionCompanies map[int64]int
	WatchedIDs          []int64
}

// RecommendationUpdater is the narrow interface TVService uses to trigger a
// recommendation-profile update after a state change, without importing the
// recommendation package's concrete type. Errors are handled internally by
// the implementation — this never fails the caller's request.
type RecommendationUpdater interface {
	ApplyTVStateChange(ctx context.Context, userID string, tvID int64)
	GetTVAffinity(ctx context.Context, userID string) (TVAffinity, error)
}

// TVService holds the business logic for the TV feature.
type TVService struct {
	repo           TVRepository
	tmdb           *tmdb.Client
	cache          cache.Cache
	recommendation RecommendationUpdater
}

// NewService constructs a TVService.
func NewService(repo TVRepository, tmdb *tmdb.Client, c cache.Cache, rec RecommendationUpdater) *TVService {
	return &TVService{repo: repo, tmdb: tmdb, cache: c, recommendation: rec}
}

func randomSeenKey(userID string) string {
	return "tv:random:seen:" + userID
}

func randomRecommendationSeenKey(userID string) string {
	return "tv:recommendations:random:seen:" + userID
}

// loadSeenIDs returns the ids previously served under the given cache key.
// Cache errors are swallowed — caching is best-effort and must never fail the request.
func (s *TVService) loadSeenIDs(ctx context.Context, key string) []int64 {
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

// saveSeenIDs merges newIDs into the seen-set, caps its size, and refreshes the TTL.
func (s *TVService) saveSeenIDs(ctx context.Context, key string, existing, newIDs []int64) {
	merged := append(existing, newIDs...)
	if len(merged) > randomSeenCap {
		merged = merged[len(merged)-randomSeenCap:]
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, key, string(raw), randomSeenTTL)
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

	results, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// TopRated returns a fixed rail of scripted series with a sufficiently large
// voting sample, ordered by their weighted rating.
func (s *TVService) TopRated(ctx context.Context) ([]TVResponse, error) {
	ids, err := s.repo.TopRatedIDs(ctx, topRatedResultLimit)
	if err != nil {
		return nil, fmt.Errorf("tv service: top rated ids: %w", err)
	}
	return s.GetByIDs(ctx, ids)
}

// Recommendations returns unwatched TV series ranked from the user's rated
// watch history. Repeated calls advance through unseen candidates.
func (s *TVService) Recommendations(ctx context.Context, userID string, page int) ([]TVResponse, int, error) {
	if s.recommendation == nil {
		return []TVResponse{}, 0, nil
	}

	aff, err := s.recommendation.GetTVAffinity(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("tv service: get tv affinity: %w", err)
	}
	criteria := RecommendationCriteria{
		GenreIDs: positiveScoreIDs(aff.Genres), KeywordIDs: positiveScoreIDs(aff.Keywords),
		CastIDs: positiveScoreIDs(aff.Actors), CreatorIDs: positiveScoreIDs(aff.Creators),
		CompanyIDs: positiveScoreIDs(aff.ProductionCompanies),
	}
	if criteria.empty() {
		return []TVResponse{}, 0, nil
	}
	candidates, err := s.repo.FindRecommendationCandidates(ctx, criteria, aff.WatchedIDs, recommendationCandidateLimit)
	if err != nil {
		return nil, 0, fmt.Errorf("tv service: find recommendation candidates: %w", err)
	}
	scored := rankByAffinity(candidates, aff)
	total := len(scored)
	start := (page - 1) * recommendationsPageSize
	if start >= total {
		return []TVResponse{}, total, nil
	}
	end := min(start+recommendationsPageSize, total)
	ids := make([]int64, end-start)
	for i, t := range scored[start:end] {
		ids[i] = t.TVID
	}
	results, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// RandomRecommendations returns a random personalized selection. It uses the
// same affinity criteria as Recommendations, but samples from eligible titles
// and remembers served ids so consecutive calls do not repeat picks.
func (s *TVService) RandomRecommendations(ctx context.Context, userID string, total int) ([]TVResponse, error) {
	return s.randomRecommendations(ctx, userID, total, true)
}

func (s *TVService) randomRecommendations(ctx context.Context, userID string, total int, resetSeenWhenExhausted bool) ([]TVResponse, error) {
	if s.recommendation == nil {
		return []TVResponse{}, nil
	}

	aff, err := s.recommendation.GetTVAffinity(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get tv affinity: %w", err)
	}
	criteria := RecommendationCriteria{
		GenreIDs: positiveScoreIDs(aff.Genres), KeywordIDs: positiveScoreIDs(aff.Keywords),
		CastIDs: positiveScoreIDs(aff.Actors), CreatorIDs: positiveScoreIDs(aff.Creators),
		CompanyIDs: positiveScoreIDs(aff.ProductionCompanies),
	}
	if criteria.empty() {
		return []TVResponse{}, nil
	}

	candidates, err := s.repo.FindRecommendationCandidates(ctx, criteria, aff.WatchedIDs, recommendationCandidateLimit)
	if err != nil {
		return nil, fmt.Errorf("tv service: find random recommendation candidates: %w", err)
	}
	seenIDs := s.loadSeenIDs(ctx, randomRecommendationSeenKey(userID))
	seenSet := make(map[int64]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		seenSet[id] = struct{}{}
	}

	eligible := make([]TV, 0, len(candidates))
	for _, candidate := range candidates {
		if _, seen := seenSet[candidate.TVID]; seen || !isAcceptableTVDoc(candidate) {
			continue
		}
		eligible = append(eligible, candidate)
	}
	rand.Shuffle(len(eligible), func(i, j int) { eligible[i], eligible[j] = eligible[j], eligible[i] })

	personalizedTotal := min(len(eligible), (total*7+9)/10)
	selectedIDs := make(map[int64]struct{}, total)
	ids := make([]int64, 0, total)
	for _, candidate := range eligible[:personalizedTotal] {
		selectedIDs[candidate.TVID] = struct{}{}
		ids = append(ids, candidate.TVID)
	}

	watchedSet := make(map[int64]struct{}, len(aff.WatchedIDs))
	for _, id := range aff.WatchedIDs {
		watchedSet[id] = struct{}{}
	}
	trendingIDs := make([]int64, 0, total-personalizedTotal)
	for page := 1; page <= randomMaxPages && len(ids)+len(trendingIDs) < total; page++ {
		trendingPage, err := s.Trending(ctx, randomTimeWindow, page)
		if err != nil {
			return nil, fmt.Errorf("tv service: random recommendation trending page %d: %w", page, err)
		}
		for _, show := range trendingPage.Results {
			if _, watched := watchedSet[show.ID]; watched {
				continue
			}
			if _, seen := seenSet[show.ID]; seen {
				continue
			}
			if _, selected := selectedIDs[show.ID]; selected {
				continue
			}
			selectedIDs[show.ID] = struct{}{}
			trendingIDs = append(trendingIDs, show.ID)
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
		if _, selected := selectedIDs[candidate.TVID]; selected {
			continue
		}
		selectedIDs[candidate.TVID] = struct{}{}
		ids = append(ids, candidate.TVID)
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	if len(ids) == 0 && resetSeenWhenExhausted && len(seenIDs) > 0 {
		_ = s.cache.Delete(ctx, randomRecommendationSeenKey(userID))
		return s.randomRecommendations(ctx, userID, total, false)
	}
	results, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	s.saveSeenIDs(ctx, randomRecommendationSeenKey(userID), seenIDs, ids)
	return results, nil
}

// discoverRecommendationIDs broadens a depleted personalized pool with
// unwatched, unseen titles from TMDB Discover, using one of the user's liked
// genres to retain relevance.
func (s *TVService) discoverRecommendationIDs(ctx context.Context, genreIDs []int64, watchedSet, seenSet, selectedIDs map[int64]struct{}, total int) ([]int64, error) {
	if total == 0 {
		return nil, nil
	}

	var genreID int64
	if len(genreIDs) > 0 {
		genreID = genreIDs[rand.Intn(len(genreIDs))]
	}
	ids := make([]int64, 0, total)
	for page := 1; page <= randomMaxPages && len(ids) < total; page++ {
		discoverPage, err := s.tmdb.Discover(ctx, "tv", genreID, page)
		if err != nil {
			return nil, fmt.Errorf("tv service: discover recommendation page %d: %w", page, err)
		}
		candidateIDs := make([]int64, 0, len(discoverPage.Results))
		for _, show := range discoverPage.Results {
			candidateIDs = append(candidateIDs, show.ID)
		}
		cached, err := s.repo.FindByTMDBIDs(ctx, candidateIDs)
		if err != nil {
			return nil, fmt.Errorf("tv service: discover recommendation cached ids: %w", err)
		}
		for _, show := range discoverPage.Results {
			if _, known := cached[show.ID]; !known {
				continue
			}
			if _, watched := watchedSet[show.ID]; watched {
				continue
			}
			if _, seen := seenSet[show.ID]; seen {
				continue
			}
			if _, selected := selectedIDs[show.ID]; selected {
				continue
			}
			selectedIDs[show.ID] = struct{}{}
			ids = append(ids, show.ID)
			if len(ids) == total {
				break
			}
		}
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids, nil
}

func positiveScoreIDs(scores map[int64]int) []int64 {
	ids := make([]int64, 0, len(scores))
	for id, score := range scores {
		if score > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetByIDs returns the same enriched response shape as Discover for a known
// set of TV IDs.
func (s *TVService) GetByIDs(ctx context.Context, ids []int64) ([]TVResponse, error) {
	if len(ids) == 0 {
		return []TVResponse{}, nil
	}

	// Load cached DB records so they can override TMDB detail (DB is the source of
	// truth for the fields it stores). Read-only during the fan-out below.
	cached, err := s.repo.FindByTMDBIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("tv service: fetch cached tv: %w", err)
	}
	knownIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := cached[id]; ok {
			knownIDs = append(knownIDs, id)
		}
	}
	if len(knownIDs) == 0 {
		return []TVResponse{}, nil
	}

	results := make([]TVResponse, len(knownIDs))
	errs := make([]error, len(knownIDs))

	var wg sync.WaitGroup
	for i, id := range knownIDs {
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
			if db, ok := cached[tvID]; ok {
				overlayDBFields(&detail, db)
			}
			results[idx] = detail
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("tv service: enrich from tmdb: %w", err)
		}
	}

	// Override next_episode_to_air.air_date with the DB value which has the
	// correct timezone (set by TVMaze sync), unlike TMDB which returns date-only.
	airDates, err := s.repo.GetNextEpisodeAirDatesByIDs(ctx, knownIDs)
	if err != nil {
		return nil, fmt.Errorf("tv service: get air dates from db: %w", err)
	}
	for i := range results {
		if airDate, ok := airDates[results[i].ID]; ok && results[i].NextEpisodeToAir != nil {
			results[i].NextEpisodeToAir.AirDate = FlexAirDate(airDate)
		}
	}

	return results, nil
}

// Random returns up to total TV series the user hasn't tracked yet (per withoutStatus),
// sampled by shuffling TMDB's trending pool (day window, already filtered through
// isAcceptableTV via Trending). Trending is paged (up to randomMaxPages) until
// enough untracked, unseen candidates are gathered or trending is exhausted. Ids
// served recently (see the Redis "seen" cache) are excluded so consecutive calls
// don't repeat the same titles.
func (s *TVService) Random(ctx context.Context, userID string, total int, withoutStatus []string) ([]TVResponse, error) {
	seenIDs := s.loadSeenIDs(ctx, randomSeenKey(userID))
	seenSet := make(map[int64]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		seenSet[id] = struct{}{}
	}

	excludeSet, err := s.trackedExcludeIDs(ctx, userID, withoutStatus)
	if err != nil {
		return nil, err
	}

	pool := make([]TVResponse, 0, total)
	poolSet := make(map[int64]struct{}, total)

	for page := 1; page <= randomMaxPages && len(pool) < total; page++ {
		trendingPage, err := s.Trending(ctx, randomTimeWindow, page)
		if err != nil {
			return nil, fmt.Errorf("tv service: random trending page %d: %w", page, err)
		}
		if len(trendingPage.Results) == 0 {
			break
		}
		for _, t := range trendingPage.Results {
			if _, ok := poolSet[t.ID]; ok {
				continue
			}
			if _, ok := excludeSet[t.ID]; ok {
				continue
			}
			if _, ok := seenSet[t.ID]; ok {
				continue
			}
			poolSet[t.ID] = struct{}{}
			pool = append(pool, t)
		}
	}

	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > total {
		pool = pool[:total]
	}

	ids := make([]int64, len(pool))
	for i, t := range pool {
		ids[i] = t.ID
	}
	s.saveSeenIDs(ctx, randomSeenKey(userID), seenIDs, ids)

	return pool, nil
}

// trackedExcludeIDs returns the set of TV ids the user has tracked with an
// account_status in withoutStatus (nil/empty withoutStatus excludes nothing).
func (s *TVService) trackedExcludeIDs(ctx context.Context, userID string, withoutStatus []string) (map[int64]struct{}, error) {
	excludeSet := make(map[int64]struct{})
	if len(withoutStatus) == 0 {
		return excludeSet, nil
	}
	statuses := make(map[string]struct{}, len(withoutStatus))
	for _, s := range withoutStatus {
		statuses[s] = struct{}{}
	}
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: tracked exclude ids: %w", err)
	}
	for _, st := range states {
		if _, ok := statuses[st.AccountStatus]; ok {
			excludeSet[st.TVID] = struct{}{}
		}
	}
	return excludeSet, nil
}

// Search finds TV series in the local catalog by name, then enriches the
// matched series with their full TMDB detail payload.
func (s *TVService) Search(ctx context.Context, query string, page int) (*response.Page[TVResponse], error) {
	ids, total, err := s.repo.SearchIDs(ctx, query, page)
	if err != nil {
		return nil, fmt.Errorf("tv service: search local catalog: %w", err)
	}
	if len(ids) == 0 {
		return &response.Page[TVResponse]{Page: page, TotalPages: max(1, (total+19)/20), TotalResults: total, Results: []TVResponse{}}, nil
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
				errs[idx] = err
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
			overlayDBFields(&details[i], db)
		}
	}
	return &response.Page[TVResponse]{Page: page, TotalPages: max(1, (total+19)/20), TotalResults: total, Results: details}, nil
}

// Trending returns TMDB's trending TV series for the given time window, enriched with
// full detail and filtered through isAcceptableTV. Cached DB records win over TMDB
// detail for the fields the DB is the source of truth for (see overlayDBFields).
func (s *TVService) Trending(ctx context.Context, timeWindow string, page int) (*response.Page[TVResponse], error) {
	trendingPage, err := s.tmdb.GetTrending(ctx, "tv", timeWindow, page)
	if err != nil {
		return nil, fmt.Errorf("tv service: trending tmdb: %w", err)
	}

	if len(trendingPage.Results) == 0 {
		return &response.Page[TVResponse]{
			Page:         trendingPage.Page,
			TotalPages:   trendingPage.TotalPages,
			TotalResults: trendingPage.TotalResults,
			Results:      []TVResponse{},
		}, nil
	}

	ids := make([]int64, len(trendingPage.Results))
	for i, r := range trendingPage.Results {
		ids[i] = r.ID
	}
	details, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("tv service: trending enrich from tmdb: %w", err)
	}

	results := make([]TVResponse, 0, len(details))
	for _, detail := range details {
		if !isAcceptableTV(detail) {
			continue
		}
		results = append(results, detail)
	}

	return &response.Page[TVResponse]{
		Page:         trendingPage.Page,
		TotalPages:   trendingPage.TotalPages,
		TotalResults: trendingPage.TotalResults,
		Results:      results,
	}, nil
}

// Hero returns exactly up to heroResultLimit currently trending series that
// exist in the local catalog and are neither watched nor waiting for the next
// episode. It scans more TMDB pages whenever filtering leaves a sparse result.
func (s *TVService) Hero(ctx context.Context, userID string) ([]TVResponse, error) {
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: hero states: %w", err)
	}
	excluded := make(map[int64]struct{}, len(states))
	for _, state := range states {
		if state.AccountStatus == "watched" || state.AccountStatus == "waiting_next_ep" {
			excluded[state.TVID] = struct{}{}
		}
	}

	results := make([]TVResponse, 0, heroResultLimit)
	selected := make(map[int64]struct{}, heroResultLimit)
	for page := 1; page <= heroMaxPages && len(results) < heroResultLimit; page++ {
		trendingPage, err := s.tmdb.GetTrending(ctx, "tv", randomTimeWindow, page)
		if err != nil {
			return nil, fmt.Errorf("tv service: hero trending page %d: %w", page, err)
		}
		if len(trendingPage.Results) == 0 {
			break
		}

		candidateIDs := make([]int64, 0, len(trendingPage.Results))
		for _, show := range trendingPage.Results {
			if _, isExcluded := excluded[show.ID]; isExcluded {
				continue
			}
			if _, alreadySelected := selected[show.ID]; alreadySelected {
				continue
			}
			candidateIDs = append(candidateIDs, show.ID)
		}
		details, err := s.GetByIDs(ctx, candidateIDs)
		if err != nil {
			return nil, fmt.Errorf("tv service: hero enrich page %d: %w", page, err)
		}
		for _, show := range details {
			if !isAcceptableTV(show) || show.BackdropPath == "" {
				continue
			}
			selected[show.ID] = struct{}{}
			results = append(results, show)
			if len(results) == heroResultLimit {
				break
			}
		}
	}

	return results, nil
}

// GetByID returns detail for a single TV series when it exists in the local
// catalog. When the series is cached in the
// DB, the DB record is the source of truth for the fields it stores (popularity,
// votes, air dates, status, type, season/episode counts, is_anime, name, poster,
// language) plus next_episode_to_air.air_date; TMDB supplies the rest (overview,
// credits, videos, genre names, watch providers). Series absent from the DB are
// not exposed. Returns tmdb.ErrNotFound if the catalog or TMDB has no such series.
func (s *TVService) GetByID(ctx context.Context, id int64) (*TVResponse, error) {
	cached, err := s.repo.FindByTMDBIDs(ctx, []int64{id})
	if err != nil {
		return nil, fmt.Errorf("tv service: fetch cached tv: %w", err)
	}
	db, ok := cached[id]
	if !ok {
		return nil, fmt.Errorf("tv service: get by id %d: %w", id, tmdb.ErrNotFound)
	}

	var detail TVResponse
	if err := s.tmdb.GetTVDetail(ctx, id, detailAppendToResponse, &detail); err != nil {
		return nil, fmt.Errorf("tv service: get by id %d: %w", id, err)
	}
	detail.MediaType = "tv"
	if detail.ImdbID == "" && detail.ExternalIDs != nil {
		detail.ImdbID = detail.ExternalIDs.ImdbID
	}

	overlayDBFields(&detail, db)

	airDates, err := s.repo.GetNextEpisodeAirDatesByIDs(ctx, []int64{id})
	if err != nil {
		return nil, fmt.Errorf("tv service: get air dates from db: %w", err)
	}
	if airDate, ok := airDates[id]; ok && detail.NextEpisodeToAir != nil {
		detail.NextEpisodeToAir.AirDate = FlexAirDate(airDate)
	}

	return &detail, nil
}

// GetSeason returns full TMDB detail for one season of a TV series, including
// its episode list. Unlike GetByID this has no DB overlay — episode watched-state
// is intentionally left to the caller, which already holds the user's full
// episode_watched array from GetStates and can compute per-episode watched state
// without a second round-trip or duplicating that logic server-side.
func (s *TVService) GetSeason(ctx context.Context, tvID int64, seasonNumber int) (*SeasonDetailResponse, error) {
	cached, err := s.repo.FindByTMDBIDs(ctx, []int64{tvID})
	if err != nil {
		return nil, fmt.Errorf("tv service: fetch cached tv for season: %w", err)
	}
	if _, ok := cached[tvID]; !ok {
		return nil, fmt.Errorf("tv service: get season %d for tv %d: %w", seasonNumber, tvID, tmdb.ErrNotFound)
	}

	var tmdbSeason struct {
		ID           int64   `json:"id"`
		Name         string  `json:"name"`
		Overview     string  `json:"overview"`
		AirDate      *string `json:"air_date"`
		PosterPath   *string `json:"poster_path"`
		SeasonNumber int     `json:"season_number"`
		VoteAverage  float64 `json:"vote_average"`
		Episodes     []struct {
			ID            int64       `json:"id"`
			Name          string      `json:"name"`
			Overview      string      `json:"overview"`
			AirDate       FlexAirDate `json:"air_date"`
			EpisodeNumber int         `json:"episode_number"`
			EpisodeType   string      `json:"episode_type"`
			SeasonNumber  int         `json:"season_number"`
			Runtime       *int        `json:"runtime"`
			StillPath     *string     `json:"still_path"`
			VoteAverage   float64     `json:"vote_average"`
			VoteCount     int         `json:"vote_count"`
		} `json:"episodes"`
	}

	if err := s.tmdb.GetTVSeason(ctx, tvID, seasonNumber, &tmdbSeason); err != nil {
		return nil, fmt.Errorf("tv service: get season %d for tv %d: %w", seasonNumber, tvID, err)
	}

	episodes := make([]SeasonEpisode, 0, len(tmdbSeason.Episodes))
	for _, ep := range tmdbSeason.Episodes {
		episodes = append(episodes, SeasonEpisode{
			ID:            ep.ID,
			Name:          ep.Name,
			Overview:      ep.Overview,
			AirDate:       ep.AirDate,
			EpisodeNumber: ep.EpisodeNumber,
			EpisodeType:   ep.EpisodeType,
			SeasonNumber:  ep.SeasonNumber,
			Runtime:       ep.Runtime,
			StillPath:     ep.StillPath,
			VoteAverage:   ep.VoteAverage,
			VoteCount:     ep.VoteCount,
		})
	}

	return &SeasonDetailResponse{
		ID:           tmdbSeason.ID,
		Name:         tmdbSeason.Name,
		Overview:     tmdbSeason.Overview,
		AirDate:      tmdbSeason.AirDate,
		PosterPath:   tmdbSeason.PosterPath,
		SeasonNumber: tmdbSeason.SeasonNumber,
		VoteAverage:  tmdbSeason.VoteAverage,
		Episodes:     episodes,
	}, nil
}

// UpsertTVState creates or updates the user's TV tracking record.
// For status="watched" it enumerates all episodes from TMDB and populates episode_watched.
func (s *TVService) UpsertTVState(ctx context.Context, userID string, req UpsertStateRequest) error {
	if req.Status != "watched" && req.Status != "watchlist" {
		return fmt.Errorf("tv service: invalid status %q", req.Status)
	}
	if req.Rating != nil && (*req.Rating < 0 || *req.Rating > 10) {
		return fmt.Errorf("tv service: invalid rating %v", *req.Rating)
	}

	if req.Status == "watchlist" {
		if err := s.repo.UpsertTVState(ctx, userID, req.ID, nil, req.Rating); err != nil {
			return err
		}
		if s.recommendation != nil {
			s.recommendation.ApplyTVStateChange(ctx, userID, req.ID)
		}
		return nil
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

	if err := s.repo.UpsertTVState(ctx, userID, req.ID, allEpisodes, req.Rating); err != nil {
		return err
	}
	if s.recommendation != nil {
		s.recommendation.ApplyTVStateChange(ctx, userID, req.ID)
	}
	return nil
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

	if err := s.repo.UpsertEpisodes(ctx, userID, req); err != nil {
		return err
	}
	if s.recommendation != nil {
		s.recommendation.ApplyTVStateChange(ctx, userID, req.ID)
	}
	return nil
}

// GetStates returns aggregated TV tracking records for the given user.
func (s *TVService) GetStates(ctx context.Context, userID string) ([]TVStateResponse, error) {
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get states: %w", err)
	}
	return states, nil
}

// UpcomingEpisodes returns watchlisted or caught-up TV series whose next
// episode has a known future air date. It is a dedicated home-rail payload,
// so clients do not need to combine discover and states responses.
func (s *TVService) UpcomingEpisodes(ctx context.Context, userID string) ([]TVStateResponse, error) {
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get upcoming episode states: %w", err)
	}

	results := make([]TVStateResponse, 0, len(states))
	for _, state := range states {
		if (state.AccountStatus != "waiting_next_ep" && state.AccountStatus != "watchlist") || state.NextEpisodeToAir == nil || string(state.NextEpisodeToAir.AirDate) == "" {
			continue
		}
		// A watchlist entry has not been started yet, so only surface a true
		// series premiere. Caught-up series can wait for any later episode.
		if state.AccountStatus == "watchlist" && (state.NextEpisodeToAir.SeasonNumber != 1 || state.NextEpisodeToAir.EpisodeNumber != 1) {
			continue
		}
		results = append(results, state)
	}

	sort.SliceStable(results, func(i, j int) bool {
		return string(results[i].NextEpisodeToAir.AirDate) < string(results[j].NextEpisodeToAir.AirDate)
	})
	if len(results) > waitingNextEpisodeResultLimit {
		results = results[:waitingNextEpisodeResultLimit]
	}

	ids := make([]int64, len(results))
	for i, state := range results {
		ids[i] = state.TVID
	}
	details, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("tv service: enrich upcoming episodes: %w", err)
	}
	detailByID := make(map[int64]TVResponse, len(details))
	for _, detail := range details {
		detailByID[detail.ID] = detail
	}
	for i := range results {
		if detail, ok := detailByID[results[i].TVID]; ok {
			results[i].Name = detail.Name
			results[i].BackdropPath = detail.BackdropPath
			results[i].PosterPath = detail.PosterPath
		}
	}

	return results, nil
}

// NowAiringWatchlist returns series the user saved before starting, once more
// than the premiere has aired.
func (s *TVService) NowAiringWatchlist(ctx context.Context, userID string) ([]TVStateResponse, error) {
	states, err := s.repo.GetStatesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("tv service: get now airing watchlist states: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	results := make([]TVStateResponse, 0, len(states))
	for _, state := range states {
		last := state.LastEpisodeToAir
		if state.AccountStatus != "watchlist" || state.TVStatus != "Returning Series" || last == nil || string(last.AirDate) == "" || string(last.AirDate) > today {
			continue
		}
		if last.SeasonNumber == 1 && last.EpisodeNumber <= 1 {
			continue
		}
		results = append(results, state)
	}

	sort.SliceStable(results, func(i, j int) bool {
		leftHasNext := results[i].NextEpisodeToAir != nil && string(results[i].NextEpisodeToAir.AirDate) != ""
		rightHasNext := results[j].NextEpisodeToAir != nil && string(results[j].NextEpisodeToAir.AirDate) != ""
		if leftHasNext != rightHasNext {
			return leftHasNext
		}
		if leftHasNext {
			return string(results[i].NextEpisodeToAir.AirDate) < string(results[j].NextEpisodeToAir.AirDate)
		}
		return string(results[i].LastEpisodeToAir.AirDate) > string(results[j].LastEpisodeToAir.AirDate)
	})
	if len(results) > waitingNextEpisodeResultLimit {
		results = results[:waitingNextEpisodeResultLimit]
	}

	ids := make([]int64, len(results))
	for i, state := range results {
		ids[i] = state.TVID
	}
	details, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("tv service: enrich now airing watchlist: %w", err)
	}
	detailByID := make(map[int64]TVResponse, len(details))
	for _, detail := range details {
		detailByID[detail.ID] = detail
	}
	for i := range results {
		if detail, ok := detailByID[results[i].TVID]; ok {
			results[i].Name = detail.Name
			results[i].BackdropPath = detail.BackdropPath
			results[i].PosterPath = detail.PosterPath
		}
	}

	return results, nil
}
