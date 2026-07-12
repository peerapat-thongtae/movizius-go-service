package recommendation

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/tv"
)

// Config holds the tunable coefficients for the scoring formula and pruning.
type Config struct {
	HalfLifeDays        float64
	RewatchBonusK       float64
	LeadActorMultiplier float64
	ActorMultiplier     float64
	DirectorMultiplier  float64
	CreatorMultiplier   float64
	PruneMinCount       int
	PruneMaxAbsScore    int
	BucketCap           int
}

// Service orchestrates recommendation-profile computation: pulling cached
// TMDB metadata and per-user tracking docs, computing pure event/aggregate
// deltas, and persisting them.
type Service struct {
	repo  Repository
	movie movie.MovieRepository
	tv    tv.TVRepository
	cfg   Config
	log   *slog.Logger
}

// NewService constructs a Service.
func NewService(repo Repository, movieRepo movie.MovieRepository, tvRepo tv.TVRepository, cfg Config, log *slog.Logger) *Service {
	return &Service{repo: repo, movie: movieRepo, tv: tvRepo, cfg: cfg, log: log}
}

// multipliers builds the bucket-weight Multipliers from the service's Config.
func (s *Service) multipliers() Multipliers {
	return Multipliers{
		LeadActor: s.cfg.LeadActorMultiplier,
		Actor:     s.cfg.ActorMultiplier,
		Director:  s.cfg.DirectorMultiplier,
		Creator:   s.cfg.CreatorMultiplier,
	}
}

// RecomputeResult reports the outcome of a full profile recompute.
type RecomputeResult struct {
	UsersProcessed int      `json:"usersProcessed"`
	Errors         []string `json:"errors,omitempty"`
}

// ApplyMovieStateChange recomputes and applies the profile delta for one
// user's movie tracking state change. Called synchronously right after the
// movie service persists a MovieUser upsert. Never returns an error to the
// caller — logs and swallows internally so a profile-update bug can never
// break a user-facing watch/rating request; the admin recompute endpoint is
// the reconciliation safety net.
func (s *Service) ApplyMovieStateChange(ctx context.Context, userID string, movieID int64) {
	if err := s.applyMovieStateChange(ctx, userID, movieID); err != nil {
		s.log.Error("recommendation: apply movie state change failed", "error", err, "user", userID, "movie", movieID)
	}
}

func (s *Service) applyMovieStateChange(ctx context.Context, userID string, movieID int64) error {
	metadata, err := s.movie.FindByTMDBIDs(ctx, []int64{movieID})
	if err != nil {
		return err
	}
	m, ok := metadata[movieID]
	if !ok {
		return nil // metadata not cached yet — nothing to score against
	}

	doc, err := s.movie.FindOne(ctx, userID, movieID)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
	}

	completion := 0.0
	if doc.WatchedAt != nil {
		completion = 1.0
	}

	prevContribution, firstTouch := previousMovieContribution(doc.ProfileContribution)
	newContribution := 0.0
	if completion > 0 {
		referenceAt := *doc.WatchedAt
		newContribution = Contribution(EventInput{
			Now:           time.Now().UTC(),
			ReferenceAt:   referenceAt,
			CompletionPct: completion,
			Rating:        doc.Rating,
			RewatchCount:  0,
			HalfLifeDays:  s.cfg.HalfLifeDays,
			RewatchBonusK: s.cfg.RewatchBonusK,
		})
	}

	delta := newContribution - prevContribution
	refs := EntityRefs{
		Genres:              m.Genres,
		Keywords:            m.Keywords,
		CastIDs:             m.CastIDs,
		DirectorID:          m.DirectorID,
		CollectionID:        m.CollectionID,
		ProductionCompanies: m.ProductionCompanies,
	}

	if delta != 0 {
		deltas := Deltas(delta, refs, s.multipliers())
		var watchedID *int64
		if firstTouch && completion > 0 {
			id := movieID
			watchedID = &id
		}
		if err := s.repo.ApplyProfileUpdate(ctx, userID, MediaTypeMovie, deltas, firstTouch && completion > 0, watchedID); err != nil {
			return err
		}
	}

	snapshot := movie.ProfileContribution{
		Contribution: newContribution,
		Applied:      (doc.ProfileContribution != nil && doc.ProfileContribution.Applied) || completion > 0,
		Version:      ScoringVersion,
	}
	return s.movie.UpdateProfileContribution(ctx, userID, movieID, snapshot)
}

// ApplyTVStateChange recomputes and applies the profile delta for one user's
// TV tracking state change. See ApplyMovieStateChange for the error-handling
// contract (never propagates errors to the caller).
func (s *Service) ApplyTVStateChange(ctx context.Context, userID string, tvID int64) {
	if err := s.applyTVStateChange(ctx, userID, tvID); err != nil {
		s.log.Error("recommendation: apply tv state change failed", "error", err, "user", userID, "tv", tvID)
	}
}

func (s *Service) applyTVStateChange(ctx context.Context, userID string, tvID int64) error {
	metadata, err := s.tv.FindByTMDBIDs(ctx, []int64{tvID})
	if err != nil {
		return err
	}
	t, ok := metadata[tvID]
	if !ok {
		return nil
	}

	doc, err := s.tv.FindOne(ctx, userID, tvID)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
	}

	completion := tvCompletionPct(doc, t)
	prevContribution, firstTouch := previousTVContribution(doc.ProfileContribution)
	newContribution := 0.0
	if completion > 0 {
		newContribution = Contribution(EventInput{
			Now:           time.Now().UTC(),
			ReferenceAt:   latestEpisodeWatchedAt(doc.EpisodeWatched),
			CompletionPct: completion,
			Rating:        doc.Rating,
			RewatchCount:  0,
			HalfLifeDays:  s.cfg.HalfLifeDays,
			RewatchBonusK: s.cfg.RewatchBonusK,
		})
	}

	delta := newContribution - prevContribution
	refs := EntityRefs{
		Genres:              t.Genres,
		Keywords:            t.Keywords,
		CastIDs:             t.CastIDs,
		CreatorIDs:          t.CreatorIDs,
		ProductionCompanies: t.ProductionCompanies,
	}

	if delta != 0 {
		deltas := Deltas(delta, refs, s.multipliers())
		var watchedID *int64
		if firstTouch && completion > 0 {
			id := tvID
			watchedID = &id
		}
		if err := s.repo.ApplyProfileUpdate(ctx, userID, MediaTypeTV, deltas, firstTouch && completion > 0, watchedID); err != nil {
			return err
		}
	}

	snapshot := tv.ProfileContribution{
		Contribution: newContribution,
		Applied:      (doc.ProfileContribution != nil && doc.ProfileContribution.Applied) || completion > 0,
		Version:      ScoringVersion,
	}
	return s.tv.UpdateProfileContribution(ctx, userID, tvID, snapshot)
}

// previousMovieContribution reads the prior contribution snapshot, reporting
// whether this is the title's first-ever contribution.
func previousMovieContribution(c *movie.ProfileContribution) (float64, bool) {
	if c == nil || !c.Applied {
		return 0, true
	}
	return c.Contribution, false
}

// previousTVContribution mirrors previousMovieContribution for TVUser docs.
func previousTVContribution(c *tv.ProfileContribution) (float64, bool) {
	if c == nil || !c.Applied {
		return 0, true
	}
	return c.Contribution, false
}

// tvCompletionPct derives a 0..1 completion ratio from episodes watched vs.
// the series' known episode count, falling back to 1.0 when the episode
// count isn't known but episodes have been watched. Known tradeoff: for
// in-production shows NumberOfEpisodes grows over time, so completion can
// understate a user's past full-completion retroactively — acceptable for v1.
func tvCompletionPct(doc *tv.TVUser, t tv.TV) float64 {
	if len(doc.EpisodeWatched) == 0 {
		return 0
	}
	if t.NumberOfEpisodes == nil || *t.NumberOfEpisodes <= 0 {
		return 1
	}
	pct := float64(len(doc.EpisodeWatched)) / float64(*t.NumberOfEpisodes)
	if pct > 1 {
		pct = 1
	}
	return pct
}

func latestEpisodeWatchedAt(episodes []tv.EpisodeWatched) time.Time {
	var latest time.Time
	for _, e := range episodes {
		if e.WatchedAt.After(latest) {
			latest = e.WatchedAt
		}
	}
	return latest
}

// GetProfile returns a user's current recommendation profile.
func (s *Service) GetProfile(ctx context.Context, userID string) (Profile, error) {
	return s.repo.GetProfile(ctx, userID)
}

// GetMovieAffinity returns a user's movie-bucket scores in the shape the
// movie package needs for ranking candidates, without exposing the full
// Profile type (movie.RecommendationUpdater declares this method to avoid
// importing this package's Profile/Bucket types).
func (s *Service) GetMovieAffinity(ctx context.Context, userID string) (movie.MovieAffinity, error) {
	profile, err := s.repo.GetProfile(ctx, userID)
	if err != nil {
		return movie.MovieAffinity{}, err
	}
	return movie.MovieAffinity{
		Genres:              bucketScores(profile.Movie.Genres),
		Keywords:            bucketScores(profile.Movie.Keywords),
		Actors:              bucketScores(profile.Movie.Actors),
		Directors:           bucketScores(profile.Movie.Directors),
		Collections:         bucketScores(profile.Movie.Collections),
		ProductionCompanies: bucketScores(profile.Movie.ProductionCompanies),
		WatchedIDs:          profile.Movie.WatchedIDs,
	}, nil
}

// bucketScores converts a Bucket (string-keyed, for Mongo storage) into an
// int64-keyed score map (for cheap lookup against TMDB entity ids).
func bucketScores(b Bucket) map[int64]int {
	scores := make(map[int64]int, len(b))
	for idStr, entry := range b {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		scores[id] = entry.Score
	}
	return scores
}

// RecomputeUser rebuilds a single user's profile from scratch by scanning
// all of their movie_user/tv_user docs plus cached metadata. Admin/repair
// operation, also used as the post-deploy backfill mechanism.
func (s *Service) RecomputeUser(ctx context.Context, userID string) error {
	profile := Profile{
		Version: ScoringVersion,
		Meta: Meta{
			DecayHalfLifeDays: s.cfg.HalfLifeDays,
			ScoringVersion:    ScoringVersion,
		},
		UpdatedAt: time.Now().UTC(),
	}
	profile.Movie = MediaProfile{
		Genres: Bucket{}, Keywords: Bucket{}, Actors: Bucket{}, Directors: Bucket{},
		Collections: Bucket{}, ProductionCompanies: Bucket{}, WatchedIDs: []int64{},
	}
	profile.TV = MediaProfile{
		Genres: Bucket{}, Keywords: Bucket{}, Actors: Bucket{}, Creators: Bucket{},
		ProductionCompanies: Bucket{}, WatchedIDs: []int64{},
	}

	movieDocs, err := s.movie.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	movieIDs := make([]int64, len(movieDocs))
	for i, d := range movieDocs {
		movieIDs[i] = d.MovieID
	}
	movieMetadata, err := s.movie.FindByTMDBIDs(ctx, movieIDs)
	if err != nil {
		return err
	}

	movieContributions := make(map[int64]float64, len(movieDocs))
	for _, d := range movieDocs {
		m, ok := movieMetadata[d.MovieID]
		if !ok || d.WatchedAt == nil {
			continue
		}
		contribution := Contribution(EventInput{
			Now:           time.Now().UTC(),
			ReferenceAt:   *d.WatchedAt,
			CompletionPct: 1.0,
			Rating:        d.Rating,
			RewatchCount:  0,
			HalfLifeDays:  s.cfg.HalfLifeDays,
			RewatchBonusK: s.cfg.RewatchBonusK,
		})
		refs := EntityRefs{
			Genres: m.Genres, Keywords: m.Keywords, CastIDs: m.CastIDs,
			DirectorID: m.DirectorID, CollectionID: m.CollectionID, ProductionCompanies: m.ProductionCompanies,
		}
		applyDeltasToProfile(&profile.Movie, Deltas(contribution, refs, s.multipliers()))
		profile.Movie.WatchedIDs = append(profile.Movie.WatchedIDs, d.MovieID)
		profile.Meta.TotalMovieWatched++
		profile.Meta.SourceEventCount++
		movieContributions[d.MovieID] = contribution
	}

	tvDocs, err := s.tv.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	tvIDs := make([]int64, len(tvDocs))
	for i, d := range tvDocs {
		tvIDs[i] = d.TVID
	}
	tvMetadata, err := s.tv.FindByTMDBIDs(ctx, tvIDs)
	if err != nil {
		return err
	}

	tvContributions := make(map[int64]float64, len(tvDocs))
	for _, d := range tvDocs {
		t, ok := tvMetadata[d.TVID]
		if !ok {
			continue
		}
		completion := tvCompletionPct(&d, t)
		if completion == 0 {
			continue
		}
		contribution := Contribution(EventInput{
			Now:           time.Now().UTC(),
			ReferenceAt:   latestEpisodeWatchedAt(d.EpisodeWatched),
			CompletionPct: completion,
			Rating:        d.Rating,
			RewatchCount:  0,
			HalfLifeDays:  s.cfg.HalfLifeDays,
			RewatchBonusK: s.cfg.RewatchBonusK,
		})
		refs := EntityRefs{
			Genres: t.Genres, Keywords: t.Keywords, CastIDs: t.CastIDs,
			CreatorIDs: t.CreatorIDs, ProductionCompanies: t.ProductionCompanies,
		}
		applyDeltasToProfile(&profile.TV, Deltas(contribution, refs, s.multipliers()))
		profile.TV.WatchedIDs = append(profile.TV.WatchedIDs, d.TVID)
		profile.Meta.TotalTvWatched++
		profile.Meta.SourceEventCount++
		tvContributions[d.TVID] = contribution
	}

	profile.Movie.Genres = PruneAndCap(profile.Movie.Genres, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.Movie.Keywords = PruneAndCap(profile.Movie.Keywords, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.Movie.Actors = PruneAndCap(profile.Movie.Actors, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.Movie.Directors = PruneAndCap(profile.Movie.Directors, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.Movie.Collections = PruneAndCap(profile.Movie.Collections, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.Movie.ProductionCompanies = PruneAndCap(profile.Movie.ProductionCompanies, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.TV.Genres = PruneAndCap(profile.TV.Genres, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.TV.Keywords = PruneAndCap(profile.TV.Keywords, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.TV.Actors = PruneAndCap(profile.TV.Actors, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.TV.Creators = PruneAndCap(profile.TV.Creators, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)
	profile.TV.ProductionCompanies = PruneAndCap(profile.TV.ProductionCompanies, s.cfg.PruneMinCount, s.cfg.PruneMaxAbsScore, s.cfg.BucketCap)

	if err := s.repo.ReplaceProfile(ctx, userID, profile); err != nil {
		return err
	}

	// Refresh each title's contribution snapshot so subsequent incremental
	// updates diff against the values baked into this recompute.
	for movieID, contribution := range movieContributions {
		snapshot := movie.ProfileContribution{Contribution: contribution, Applied: true, Version: ScoringVersion}
		if err := s.movie.UpdateProfileContribution(ctx, userID, movieID, snapshot); err != nil {
			return err
		}
	}
	for tvID, contribution := range tvContributions {
		snapshot := tv.ProfileContribution{Contribution: contribution, Applied: true, Version: ScoringVersion}
		if err := s.tv.UpdateProfileContribution(ctx, userID, tvID, snapshot); err != nil {
			return err
		}
	}

	return nil
}

// applyDeltasToProfile applies a set of first-touch deltas directly to a
// from-scratch MediaProfile being built during a full recompute (every
// delta here is necessarily a first-touch for its title, so count always
// increments by 1).
func applyDeltasToProfile(mp *MediaProfile, deltas []BucketDelta) {
	for _, d := range deltas {
		b := bucketByNamePtr(mp, d.Bucket)
		if b == nil {
			continue
		}
		if *b == nil {
			*b = Bucket{}
		}
		key := entityKey(d.EntityID)
		entry := (*b)[key]
		entry.RawSum += d.RawSumDelta
		entry.Count++
		entry.Score = Score(entry.RawSum, entry.Count)
		(*b)[key] = entry
	}
}

func bucketByNamePtr(mp *MediaProfile, name string) *Bucket {
	switch name {
	case BucketGenres:
		return &mp.Genres
	case BucketKeywords:
		return &mp.Keywords
	case BucketActors:
		return &mp.Actors
	case BucketDirectors:
		return &mp.Directors
	case BucketCreators:
		return &mp.Creators
	case BucketCollections:
		return &mp.Collections
	case BucketProductionCompanies:
		return &mp.ProductionCompanies
	default:
		return nil
	}
}

// RecomputeAll rebuilds every user's recommendation profile from scratch.
// Synchronous/blocking admin/repair operation — use after changing the
// scoring formula/version or half-life, following the datasync package's
// existing synchronous-endpoint precedent.
func (s *Service) RecomputeAll(ctx context.Context) (RecomputeResult, error) {
	movieDocs, err := s.movie.FindAllMovieUserDocs(ctx)
	if err != nil {
		return RecomputeResult{}, err
	}
	tvDocs, err := s.tv.FindAllTVUserDocs(ctx)
	if err != nil {
		return RecomputeResult{}, err
	}

	userIDs := map[string]struct{}{}
	for _, d := range movieDocs {
		userIDs[d.UserID] = struct{}{}
	}
	for _, d := range tvDocs {
		userIDs[d.UserID] = struct{}{}
	}

	result := RecomputeResult{}
	for userID := range userIDs {
		if err := s.RecomputeUser(ctx, userID); err != nil {
			result.Errors = append(result.Errors, userID+": "+err.Error())
			continue
		}
		result.UsersProcessed++
	}
	return result, nil
}
