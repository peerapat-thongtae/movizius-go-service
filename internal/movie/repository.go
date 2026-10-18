package movie

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/peera/movizius-go-service/internal/shared/bsonutil"
)

// MovieRepository is the data access contract for the movie collections.
type MovieRepository interface {
	FindByUserID(ctx context.Context, userID string) ([]MovieUser, error)
	FindByTMDBIDs(ctx context.Context, ids []int64) (map[int64]Movie, error)
	FindTrackedIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error)
	SearchIDs(ctx context.Context, query string, page int) (ids []int64, total int, err error)
	DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) (ids []int64, total int, err error)
	TopRatedIDs(ctx context.Context, limit int) ([]int64, error)
	UpsertState(ctx context.Context, userID string, req UpsertStateRequest) error
	UpsertDetail(ctx context.Context, data MovieResponse) error
	DeleteByTMDBID(ctx context.Context, id int64) error
	ReconcilePopularity(ctx context.Context, export map[int64]float64, maxDeleteRatio float64) (ReconcileResult, error)
	PruneUnacceptableUntracked(ctx context.Context, dryRun bool) ([]PruneCandidate, error)
	// FindOne returns a single user's tracking doc for a movie, or (nil, nil) if none exists.
	FindOne(ctx context.Context, userID string, movieID int64) (*MovieUser, error)
	// UpdateProfileContribution persists the recommendation-profile snapshot for a user's movie doc.
	UpdateProfileContribution(ctx context.Context, userID string, movieID int64, c ProfileContribution) error
	// FindAllMovieUserDocs returns every movie_user doc (for recommendation profile full recompute).
	FindAllMovieUserDocs(ctx context.Context) ([]MovieUser, error)
	// FindRecommendationCandidates returns cached movies matching at least one
	// of the given entity ids (OR across non-empty buckets), excluding
	// excludeIDs, capped at limit. Returns (nil, nil) when every criteria
	// list is empty (nothing to match against).
	FindRecommendationCandidates(ctx context.Context, criteria RecommendationCriteria, excludeIDs []int64, limit int) ([]Movie, error)
}

// RecommendationCriteria carries the entity ids to match candidates against,
// one list per bucket used in the recommendation profile.
type RecommendationCriteria struct {
	GenreIDs      []int64
	KeywordIDs    []int64
	CastIDs       []int64
	DirectorIDs   []int64
	CollectionIDs []int64
	CompanyIDs    []int64
}

// PruneCandidate is a catalog movie selected by the unacceptable-data cleanup.
// Candidates are ordered by popularity descending for dry-run review.
type PruneCandidate struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Popularity float64 `json:"popularity"`
}

// empty reports whether every bucket in the criteria is empty.
func (c RecommendationCriteria) empty() bool {
	return len(c.GenreIDs) == 0 && len(c.KeywordIDs) == 0 && len(c.CastIDs) == 0 &&
		len(c.DirectorIDs) == 0 && len(c.CollectionIDs) == 0 && len(c.CompanyIDs) == 0
}

type mongoMovieRepository struct {
	db *mongo.Database
}

const (
	topRatedMinimumVotes    int64   = 20000
	topRatedFallbackAverage float64 = 6.5
)

// NewRepository constructs a MovieRepository backed by MongoDB.
func NewRepository(db *mongo.Database) MovieRepository {
	return &mongoMovieRepository{db: db}
}

func (r *mongoMovieRepository) FindByUserID(ctx context.Context, userID string) ([]MovieUser, error) {
	coll := r.db.Collection("movie_user")

	cursor, err := coll.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("movie: find by user_id: %w", err)
	}
	defer cursor.Close(ctx)

	results := []MovieUser{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("movie: decode results: %w", err)
	}
	return results, nil
}

func (r *mongoMovieRepository) FindByTMDBIDs(ctx context.Context, ids []int64) (map[int64]Movie, error) {
	cursor, err := r.db.Collection("movie").Find(ctx, bson.M{"id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("movie: find by tmdb ids: %w", err)
	}
	defer cursor.Close(ctx)

	result := make(map[int64]Movie, len(ids))
	for cursor.Next(ctx) {
		var m Movie
		if err := cursor.Decode(&m); err != nil {
			return nil, fmt.Errorf("movie: decode movie: %w", err)
		}
		result[m.MovieID] = m
	}
	return result, cursor.Err()
}

// TopRatedIDs returns proven, released catalog movies ranked with a Bayesian
// weighted rating. The score accounts for both vote average and vote count, so
// a score from a smaller sample cannot outrank a similarly-rated title with a
// much larger audience.
func (r *mongoMovieRepository) TopRatedIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 {
		return []int64{}, nil
	}

	baseFilter := bson.D{
		{Key: "adult", Value: bson.D{{Key: "$ne", Value: true}}},
		{Key: "status", Value: "Released"},
		{Key: "poster_path", Value: bson.D{
			{Key: "$type", Value: "string"},
			{Key: "$ne", Value: ""},
		}},
		{Key: "vote_average", Value: bson.D{{Key: "$gt", Value: 0}}},
	}

	// C is the catalogue's mean rating. It anchors titles with fewer votes
	// toward a realistic baseline instead of trusting their raw rating alone.
	baselinePipeline := mongo.Pipeline{
		{{Key: "$match", Value: append(append(bson.D{}, baseFilter...), bson.E{Key: "vote_count", Value: bson.D{{Key: "$gt", Value: 0}}})}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "average", Value: bson.D{{Key: "$avg", Value: "$vote_average"}}}}}},
	}
	baselineCursor, err := r.db.Collection("movie").Aggregate(ctx, baselinePipeline)
	if err != nil {
		return nil, fmt.Errorf("movie: calculate top rated baseline: %w", err)
	}
	defer baselineCursor.Close(ctx)

	baseline := topRatedFallbackAverage
	if baselineCursor.Next(ctx) {
		var result struct {
			Average float64 `bson:"average"`
		}
		if err := baselineCursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("movie: decode top rated baseline: %w", err)
		}
		if result.Average > 0 {
			baseline = result.Average
		}
	}
	if err := baselineCursor.Err(); err != nil {
		return nil, fmt.Errorf("movie: iterate top rated baseline: %w", err)
	}

	filter := append(append(bson.D{}, baseFilter...), bson.E{Key: "vote_count", Value: bson.D{{Key: "$gt", Value: topRatedMinimumVotes}}})
	voteDenominator := bson.D{{Key: "$add", Value: bson.A{"$vote_count", topRatedMinimumVotes}}}
	weightedScore := bson.D{{Key: "$add", Value: bson.A{
		bson.D{{Key: "$multiply", Value: bson.A{
			bson.D{{Key: "$divide", Value: bson.A{"$vote_count", voteDenominator}}},
			"$vote_average",
		}}},
		bson.D{{Key: "$multiply", Value: bson.A{
			bson.D{{Key: "$divide", Value: bson.A{topRatedMinimumVotes, voteDenominator}}},
			baseline,
		}}},
	}}}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$set", Value: bson.D{{Key: "weighted_score", Value: weightedScore}}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "weighted_score", Value: -1},
			{Key: "vote_count", Value: -1},
			{Key: "popularity", Value: -1},
			{Key: "id", Value: 1},
		}}},
		{{Key: "$limit", Value: int64(limit)}},
		{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "id", Value: 1}}}},
	}
	cursor, err := r.db.Collection("movie").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("movie: aggregate top rated ids: %w", err)
	}
	defer cursor.Close(ctx)

	ids := make([]int64, 0, limit)
	for cursor.Next(ctx) {
		var item struct {
			ID int64 `bson:"id"`
		}
		if err := cursor.Decode(&item); err != nil {
			return nil, fmt.Errorf("movie: decode top rated id: %w", err)
		}
		ids = append(ids, item.ID)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("movie: iterate top rated ids: %w", err)
	}
	return ids, nil
}

// FindTrackedIDs returns the requested IDs that appear in at least one user's movie list.
func (r *mongoMovieRepository) FindTrackedIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	tracked := make(map[int64]struct{})
	if len(ids) == 0 {
		return tracked, nil
	}

	values, err := r.db.Collection("movie_user").Distinct(ctx, "id", bson.M{"id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("movie: find tracked ids: %w", err)
	}
	for _, value := range values {
		switch id := value.(type) {
		case int64:
			tracked[id] = struct{}{}
		case int32:
			tracked[int64(id)] = struct{}{}
		}
	}
	return tracked, nil
}

// SearchIDs ranks local title matches as exact, prefix, then contains matches.
func (r *mongoMovieRepository) SearchIDs(ctx context.Context, query string, page int) ([]int64, int, error) {
	pipeline := buildTitleSearchPipeline(query, page, "title", "original_title")
	cursor, err := r.db.Collection("movie").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, fmt.Errorf("movie: search aggregate: %w", err)
	}
	defer cursor.Close(ctx)
	var result []searchFacetResult
	if err := cursor.All(ctx, &result); err != nil {
		return nil, 0, fmt.Errorf("movie: decode search: %w", err)
	}
	return searchFacet(result)
}

type searchFacetResult struct {
	Metadata []struct {
		Total int `bson:"total"`
	} `bson:"metadata"`
	Data []struct {
		ID int64 `bson:"id"`
	} `bson:"data"`
}

func searchFacet(result []searchFacetResult) ([]int64, int, error) {
	if len(result) == 0 {
		return nil, 0, nil
	}
	ids := make([]int64, 0, len(result[0].Data))
	for _, item := range result[0].Data {
		ids = append(ids, item.ID)
	}
	if len(result[0].Metadata) == 0 {
		return ids, 0, nil
	}
	return ids, result[0].Metadata[0].Total, nil
}

func buildTitleSearchPipeline(query string, page int, titleField, originalTitleField string) bson.A {
	const pageSize = 20
	quoted := regexp.QuoteMeta(strings.TrimSpace(query))
	matchAny := func(pattern string) bson.D {
		return bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: titleField, Value: bson.D{{Key: "$regex", Value: pattern}, {Key: "$options", Value: "i"}}}},
			bson.D{{Key: originalTitleField, Value: bson.D{{Key: "$regex", Value: pattern}, {Key: "$options", Value: "i"}}}},
		}}}
	}
	// $switch.case is an aggregation expression, so it must use $regexMatch
	// rather than the $regex query operator used by the initial $match stage.
	matchesExpression := func(pattern string) bson.D {
		return bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "$regexMatch", Value: bson.D{{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$" + titleField, ""}}}}, {Key: "regex", Value: pattern}, {Key: "options", Value: "i"}}}},
			bson.D{{Key: "$regexMatch", Value: bson.D{{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$" + originalTitleField, ""}}}}, {Key: "regex", Value: pattern}, {Key: "options", Value: "i"}}}},
		}}}
	}
	rank := bson.D{{Key: "$switch", Value: bson.D{
		{Key: "branches", Value: bson.A{
			bson.D{{Key: "case", Value: matchesExpression("^" + quoted + "$")}, {Key: "then", Value: 3}},
			bson.D{{Key: "case", Value: matchesExpression("^" + quoted)}, {Key: "then", Value: 2}},
		}},
		{Key: "default", Value: 1},
	}}}
	return bson.A{
		bson.D{{Key: "$match", Value: matchAny(quoted)}},
		bson.D{{Key: "$addFields", Value: bson.D{{Key: "_search_rank", Value: rank}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_search_rank", Value: -1}, {Key: "popularity", Value: -1}, {Key: "vote_count", Value: -1}, {Key: "id", Value: 1}}}},
		bson.D{{Key: "$facet", Value: bson.D{
			{Key: "metadata", Value: bson.A{bson.D{{Key: "$count", Value: "total"}}}},
			{Key: "data", Value: bson.A{
				bson.D{{Key: "$skip", Value: (page - 1) * pageSize}},
				bson.D{{Key: "$limit", Value: pageSize}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "id", Value: 1}}}},
			}},
		}}},
	}
}

func (r *mongoMovieRepository) UpsertState(ctx context.Context, userID string, req UpsertStateRequest) error {
	now := time.Now().UTC()
	filter := bson.M{"id": req.ID, "user_id": userID}

	var update any
	if req.Status == "watched" {
		// Pipeline update: preserve an existing watched_at instead of overwriting it.
		// $setOnInsert can't be mixed with a pipeline, so watchlisted_at/media_type
		// use $ifNull to only take effect when the document is first created.
		set := bson.M{
			"updated_at":     now,
			"watched_at":     bson.M{"$ifNull": bson.A{"$watched_at", now}},
			"watchlisted_at": bson.M{"$ifNull": bson.A{"$watchlisted_at", now}},
			"media_type":     bson.M{"$ifNull": bson.A{"$media_type", "movie"}},
		}
		if req.Rating != nil {
			set["rating"] = *req.Rating
		}
		update = mongo.Pipeline{bson.D{{Key: "$set", Value: set}}}
	} else {
		set := bson.M{"updated_at": now}
		if req.Rating != nil {
			set["rating"] = *req.Rating
		}
		update = bson.M{
			"$set":         set,
			"$unset":       bson.M{"watched_at": ""},
			"$setOnInsert": bson.M{"watchlisted_at": now, "media_type": "movie"},
		}
	}

	_, err := r.db.Collection("movie_user").UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("movie: upsert state: %w", err)
	}
	return nil
}

func (r *mongoMovieRepository) FindOne(ctx context.Context, userID string, movieID int64) (*MovieUser, error) {
	var m MovieUser
	err := r.db.Collection("movie_user").FindOne(ctx, bson.M{"id": movieID, "user_id": userID}).Decode(&m)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("movie: find one %d/%s: %w", movieID, userID, err)
	}
	return &m, nil
}

func (r *mongoMovieRepository) UpdateProfileContribution(ctx context.Context, userID string, movieID int64, c ProfileContribution) error {
	filter := bson.M{"id": movieID, "user_id": userID}
	update := bson.M{"$set": bson.M{"profile_contribution": c}}
	if _, err := r.db.Collection("movie_user").UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("movie: update profile contribution %d/%s: %w", movieID, userID, err)
	}
	return nil
}

func (r *mongoMovieRepository) FindAllMovieUserDocs(ctx context.Context) ([]MovieUser, error) {
	cursor, err := r.db.Collection("movie_user").Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("movie: find all movie_user docs: %w", err)
	}
	defer cursor.Close(ctx)

	results := []MovieUser{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("movie: decode all movie_user docs: %w", err)
	}
	return results, nil
}

func (r *mongoMovieRepository) FindRecommendationCandidates(ctx context.Context, criteria RecommendationCriteria, excludeIDs []int64, limit int) ([]Movie, error) {
	if criteria.empty() {
		return nil, nil
	}

	var or bson.A
	if len(criteria.GenreIDs) > 0 {
		or = append(or, bson.M{"genres": bson.M{"$in": criteria.GenreIDs}})
	}
	if len(criteria.KeywordIDs) > 0 {
		or = append(or, bson.M{"keywords": bson.M{"$in": criteria.KeywordIDs}})
	}
	if len(criteria.CastIDs) > 0 {
		or = append(or, bson.M{"cast_ids": bson.M{"$in": criteria.CastIDs}})
	}
	if len(criteria.DirectorIDs) > 0 {
		or = append(or, bson.M{"director_id": bson.M{"$in": criteria.DirectorIDs}})
	}
	if len(criteria.CollectionIDs) > 0 {
		or = append(or, bson.M{"collection_id": bson.M{"$in": criteria.CollectionIDs}})
	}
	if len(criteria.CompanyIDs) > 0 {
		or = append(or, bson.M{"production_companies": bson.M{"$in": criteria.CompanyIDs}})
	}

	filter := bson.M{"$or": or}
	if len(excludeIDs) > 0 {
		filter["id"] = bson.M{"$nin": excludeIDs}
	}

	cursor, err := r.db.Collection("movie").Find(ctx, filter, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("movie: find recommendation candidates: %w", err)
	}
	defer cursor.Close(ctx)

	results := []Movie{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("movie: decode recommendation candidates: %w", err)
	}
	return results, nil
}

func (r *mongoMovieRepository) DeleteByTMDBID(ctx context.Context, id int64) error {
	filter := bson.M{"id": id}
	if _, err := r.db.Collection("movie").DeleteOne(ctx, filter); err != nil {
		return fmt.Errorf("movie: delete movie %d: %w", id, err)
	}
	if _, err := r.db.Collection("movie_user").DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("movie: delete movie_user %d: %w", id, err)
	}
	return nil
}

// PruneUnacceptableUntracked deletes movie documents that fail the acceptability filter and are not
// tracked by any user (absent from movie_user). It returns the selected movies; when dryRun is
// true it returns the same candidates without deleting anything.
func (r *mongoMovieRepository) PruneUnacceptableUntracked(ctx context.Context, dryRun bool) ([]PruneCandidate, error) {
	trackedRaw, err := r.db.Collection("movie_user").Distinct(ctx, "id", bson.M{})
	if err != nil {
		return nil, fmt.Errorf("movie: prune distinct tracked ids: %w", err)
	}
	tracked := make(map[int64]struct{}, len(trackedRaw))
	for _, v := range trackedRaw {
		switch id := v.(type) {
		case int64:
			tracked[id] = struct{}{}
		case int32:
			tracked[int64(id)] = struct{}{}
		}
	}

	projection := bson.M{"_id": 0, "id": 1, "title": 1, "popularity": 1, "vote_count": 1, "poster_path": 1, "genres": 1, "original_language": 1, "status": 1}
	cursor, err := r.db.Collection("movie").Find(ctx, bson.M{}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, fmt.Errorf("movie: prune find: %w", err)
	}
	defer cursor.Close(ctx)

	var candidates []PruneCandidate
	for cursor.Next(ctx) {
		var m Movie
		if err := cursor.Decode(&m); err != nil {
			return nil, fmt.Errorf("movie: prune decode: %w", err)
		}
		if _, isTracked := tracked[m.MovieID]; isTracked {
			continue
		}
		if !isAcceptableMovieDoc(m) {
			var popularity float64
			if m.Popularity != nil {
				popularity = *m.Popularity
			}
			candidates = append(candidates, PruneCandidate{ID: m.MovieID, Title: m.Title, Popularity: popularity})
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("movie: prune cursor: %w", err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Popularity != candidates[j].Popularity {
			return candidates[i].Popularity > candidates[j].Popularity
		}
		return candidates[i].ID < candidates[j].ID
	})
	if dryRun {
		return candidates, nil
	}

	toDelete := make([]int64, len(candidates))
	for i, candidate := range candidates {
		toDelete[i] = candidate.ID
	}

	for start := 0; start < len(toDelete); start += reconcileDeleteBatch {
		end := start + reconcileDeleteBatch
		if end > len(toDelete) {
			end = len(toDelete)
		}
		filter := bson.M{"id": bson.M{"$in": toDelete[start:end]}}
		if _, err := r.db.Collection("movie").DeleteMany(ctx, filter); err != nil {
			return nil, fmt.Errorf("movie: prune delete movie: %w", err)
		}
		if _, err := r.db.Collection("movie_user").DeleteMany(ctx, filter); err != nil {
			return nil, fmt.Errorf("movie: prune delete movie_user: %w", err)
		}
	}

	return candidates, nil
}

// reconcileUpdateBatch is the number of popularity updates flushed per BulkWrite.
const reconcileUpdateBatch = 1000

// reconcileDeleteBatch is the number of ids deleted per DeleteMany call.
const reconcileDeleteBatch = 1000

// ReconcilePopularity walks the movie collection against the TMDB daily export.
// For movies present in the export it updates popularity when it differs; movies
// absent from the export are deleted (cascading to movie_user, mirroring
// DeleteByTMDBID). If the delete set is implausibly large — more than
// maxDeleteRatio of the scanned docs, or the export is empty — the delete phase
// is skipped (popularity updates are still applied) to guard against a partial
// or corrupt download. No documents are ever inserted.
func (r *mongoMovieRepository) ReconcilePopularity(ctx context.Context, export map[int64]float64, maxDeleteRatio float64) (ReconcileResult, error) {
	res := ReconcileResult{ExistingIDs: make(map[int64]struct{})}

	projection := bson.M{"_id": 0, "id": 1, "popularity": 1}
	cursor, err := r.db.Collection("movie").Find(ctx, bson.M{}, options.Find().SetProjection(projection))
	if err != nil {
		return res, fmt.Errorf("movie: reconcile find: %w", err)
	}
	defer cursor.Close(ctx)

	updates := make([]mongo.WriteModel, 0, reconcileUpdateBatch)
	var toDelete []int64

	flushUpdates := func() error {
		if len(updates) == 0 {
			return nil
		}
		out, err := r.db.Collection("movie").BulkWrite(ctx, updates, options.BulkWrite().SetOrdered(false))
		if err != nil {
			return fmt.Errorf("movie: reconcile bulk update: %w", err)
		}
		res.Updated += out.ModifiedCount
		updates = updates[:0]
		return nil
	}

	for cursor.Next(ctx) {
		var doc struct {
			ID         int64    `bson:"id"`
			Popularity *float64 `bson:"popularity"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return res, fmt.Errorf("movie: reconcile decode: %w", err)
		}
		res.Scanned++
		res.ExistingIDs[doc.ID] = struct{}{}

		pop, ok := export[doc.ID]
		if !ok {
			toDelete = append(toDelete, doc.ID)
			continue
		}
		if doc.Popularity == nil || *doc.Popularity != pop {
			updates = append(updates, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"id": doc.ID}).
				SetUpdate(bson.M{"$set": bson.M{"popularity": pop}}))
			if len(updates) >= reconcileUpdateBatch {
				if err := flushUpdates(); err != nil {
					return res, err
				}
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return res, fmt.Errorf("movie: reconcile cursor: %w", err)
	}
	if err := flushUpdates(); err != nil {
		return res, err
	}

	// Safety guard: bail out of deletes on a suspiciously large removal set or an
	// empty export (both signal a bad download), but keep the popularity updates.
	if len(export) == 0 || float64(len(toDelete)) > maxDeleteRatio*float64(res.Scanned) {
		res.SkippedDelete = true
		return res, nil
	}

	for start := 0; start < len(toDelete); start += reconcileDeleteBatch {
		end := start + reconcileDeleteBatch
		if end > len(toDelete) {
			end = len(toDelete)
		}
		batch := toDelete[start:end]
		filter := bson.M{"id": bson.M{"$in": batch}}

		out, err := r.db.Collection("movie").DeleteMany(ctx, filter)
		if err != nil {
			return res, fmt.Errorf("movie: reconcile delete movie: %w", err)
		}
		res.Deleted += out.DeletedCount
		if _, err := r.db.Collection("movie_user").DeleteMany(ctx, filter); err != nil {
			return res, fmt.Errorf("movie: reconcile delete movie_user: %w", err)
		}
	}

	return res, nil
}

func (r *mongoMovieRepository) UpsertDetail(ctx context.Context, data MovieResponse) error {
	now := time.Now().UTC()
	filter := bson.M{"id": data.ID}
	model := movieToModel(data, now)
	update := bson.M{
		"$set": bsonutil.StructToBsonM(model, "_id", "id", "vote_average", "vote_count"),
		"$unset": bson.M{
			"on_screen_cast_ids": "",
			"writer_ids":         "",
		},
		// vote_average and vote_count are owned by IMDB sync; only set on first insert.
		"$setOnInsert": bson.M{
			"vote_average": data.VoteAverage,
			"vote_count":   data.VoteCount,
		},
	}
	_, err := r.db.Collection("movie").UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("movie: upsert detail: %w", err)
	}
	return nil
}

// DiscoverIDs returns a paginated list of TMDB movie IDs matching the query,
// along with the total count of matching documents.
func (r *mongoMovieRepository) DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) ([]int64, int, error) {
	// When filtering by account_status, start from movie_user (small, user-specific set)
	// and join into movie. This avoids scanning the full movie collection.
	var (
		coll     *mongo.Collection
		pipeline bson.A
	)
	if len(q.WithAccountStatus) > 0 && userID != "" {
		coll = r.db.Collection("movie_user")
		pipeline = buildAccountStatusPipeline(userID, q)
	} else {
		coll = r.db.Collection("movie")
		pipeline = buildDiscoverPipeline(userID, q)
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, fmt.Errorf("movie: discover aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	var facetResult []struct {
		Metadata []struct {
			Total int `bson:"total"`
		} `bson:"metadata"`
		Data []struct {
			ID int64 `bson:"id"`
		} `bson:"data"`
	}
	if err := cursor.All(ctx, &facetResult); err != nil {
		return nil, 0, fmt.Errorf("movie: decode discover: %w", err)
	}
	if len(facetResult) == 0 {
		return nil, 0, nil
	}

	total := 0
	if len(facetResult[0].Metadata) > 0 {
		total = facetResult[0].Metadata[0].Total
	}

	ids := make([]int64, 0, len(facetResult[0].Data))
	for _, d := range facetResult[0].Data {
		ids = append(ids, d.ID)
	}
	return ids, total, nil
}

// buildDiscoverPipeline runs on the movie collection (no account_status filter).
func buildDiscoverPipeline(userID string, q DiscoverQuery) bson.A {
	const pageSize = 20
	skip := (q.Page - 1) * pageSize

	pipeline := bson.A{}
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: movieMatchConditions(q)}})
	if sortByReleaseDate(q.SortBy) {
		pipeline = append(pipeline, effectiveReleaseDateStage())
	}

	// Left join movie_user when filtering out a status or sorting by user fields.
	if (sortByUserField(q.SortBy) || len(q.WithoutAccountStatus) > 0) && userID != "" {
		pipeline = append(pipeline,
			bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: "movie_user"},
				{Key: "localField", Value: "id"},
				{Key: "foreignField", Value: "id"},
				{Key: "pipeline", Value: bson.A{
					bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},
				}},
				{Key: "as", Value: "_user"},
			}}},
			bson.D{{Key: "$unwind", Value: bson.D{
				{Key: "path", Value: "$_user"},
				{Key: "preserveNullAndEmptyArrays", Value: true},
			}}},
			bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "watched_at", Value: "$_user.watched_at"},
				{Key: "watchlisted_at", Value: "$_user.watchlisted_at"},
			}}},
		)
	}
	if len(q.WithoutAccountStatus) > 0 && userID != "" {
		pipeline = append(pipeline,
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "_account_status", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$ifNull", Value: bson.A{"$_user.watched_at", false}}},
				"watched",
				"watchlist",
			}}}}}}},
			bson.D{{Key: "$match", Value: accountStatusMatchCond(nil, q.WithoutAccountStatus)}},
		)
	}

	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortStage(q.SortBy)}})
	pipeline = append(pipeline, watchProviderStages(q.WatchRegion, q.WithWatchProviders)...)
	pipeline = append(pipeline, discoverFacet(skip, pageSize))
	return pipeline
}

// sortByUserField reports whether the sort requires joining movie_user.
func sortByUserField(sortBy string) bool {
	s := strings.ToLower(sortBy)
	return strings.HasPrefix(s, "watched_at") || strings.HasPrefix(s, "watchlisted_at")
}

// buildAccountStatusPipeline runs on movie_user so it starts from a small,
// user-scoped set before joining the full movie collection.
func buildAccountStatusPipeline(userID string, q DiscoverQuery) bson.A {
	const pageSize = 20
	skip := (q.Page - 1) * pageSize

	pipeline := bson.A{
		// 1. Small initial set: only this user's movie entries.
		bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},

		// 2. Derive account_status from watched_at.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "_account_status", Value: bson.D{
				{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$ifNull", Value: bson.A{"$watched_at", false}}},
					"watched",
					"watchlist",
				}},
			}},
		}}},

		// 3. Filter to requested status before the join — keeps the join set tiny.
		bson.D{{Key: "$match", Value: accountStatusMatchCond(q.WithAccountStatus, q.WithoutAccountStatus)}},

		// 4. Join movie details (only for the filtered set, not the whole collection).
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "movie"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "as", Value: "_movie"},
		}}},
		bson.D{{Key: "$unwind", Value: "$_movie"}},

		// 5. Promote movie fields to root; preserve user-derived fields.
		bson.D{{Key: "$replaceRoot", Value: bson.D{
			{Key: "newRoot", Value: bson.D{{Key: "$mergeObjects", Value: bson.A{
				"$_movie",
				bson.D{
					{Key: "_account_status", Value: "$_account_status"},
					{Key: "watched_at", Value: "$watched_at"},
					{Key: "watchlisted_at", Value: "$watchlisted_at"},
				},
			}}}},
		}}},
	}

	// 6. Optional movie filters (applied after merge so field names match movie schema).
	if cond := movieMatchConditions(q); len(cond) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: cond}})
	}
	if sortByReleaseDate(q.SortBy) {
		pipeline = append(pipeline, effectiveReleaseDateStage())
	}

	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortStage(q.SortBy)}})
	pipeline = append(pipeline, watchProviderStages(q.WatchRegion, q.WithWatchProviders)...)
	pipeline = append(pipeline, discoverFacet(skip, pageSize))
	return pipeline
}

// accountStatusMatchCond returns a $match filter for _account_status using $in / $nin.
func accountStatusMatchCond(with, without []string) bson.D {
	cond := bson.D{}
	if len(with) > 0 {
		cond = append(cond, bson.E{Key: "_account_status", Value: bson.D{{Key: "$in", Value: with}}})
	}
	if len(without) > 0 {
		cond = append(cond, bson.E{Key: "_account_status", Value: bson.D{{Key: "$nin", Value: without}}})
	}
	return cond
}

// movieMatchConditions builds $match conditions for fields on the movie collection.
func movieMatchConditions(q DiscoverQuery) bson.D {
	match := bson.D{}
	if !q.IncludeAdult {
		// $ne: true matches both missing-field and explicit false docs.
		match = append(match, bson.E{Key: "adult", Value: bson.D{{Key: "$ne", Value: true}}})
	}
	if len(q.WithGenres) > 0 {
		match = append(match, bson.E{Key: "genres", Value: bson.D{{Key: "$all", Value: q.WithGenres}}})
	}
	if len(q.WithoutGenres) > 0 {
		match = append(match, bson.E{Key: "genres", Value: bson.D{{Key: "$nin", Value: q.WithoutGenres}}})
	}
	if q.PrimaryReleaseYear > 0 {
		y := fmt.Sprintf("%04d", q.PrimaryReleaseYear)
		match = append(match, bson.E{Key: "release_date", Value: bson.D{
			{Key: "$gte", Value: y + "-01-01"},
			{Key: "$lte", Value: y + "-12-31"},
		}})
	} else if q.ReleaseDateGte != "" || q.ReleaseDateLte != "" {
		// Use release_date_th when non-empty, fall back to release_date.
		// Normalize input: gte trims to YYYY-MM-DD; lte appends "Z" so RFC3339 values on that day pass.
		effectiveDate := effectiveReleaseDateExpression()
		var arms bson.A
		if q.ReleaseDateGte != "" {
			arms = append(arms, bson.D{{Key: "$gte", Value: bson.A{effectiveDate, normalizeDateGte(q.ReleaseDateGte)}}})
		}
		if q.ReleaseDateLte != "" {
			arms = append(arms, bson.D{{Key: "$lte", Value: bson.A{effectiveDate, normalizeDateLte(q.ReleaseDateLte)}}})
		}
		var expr any
		if len(arms) == 1 {
			expr = arms[0]
		} else {
			expr = bson.D{{Key: "$and", Value: arms}}
		}
		match = append(match, bson.E{Key: "$expr", Value: expr})
	}
	if q.VoteAverageGte > 0 || q.VoteAverageLte > 0 {
		voteAvg := bson.D{}
		if q.VoteAverageGte > 0 {
			voteAvg = append(voteAvg, bson.E{Key: "$gte", Value: q.VoteAverageGte})
		}
		if q.VoteAverageLte > 0 {
			voteAvg = append(voteAvg, bson.E{Key: "$lte", Value: q.VoteAverageLte})
		}
		match = append(match, bson.E{Key: "vote_average", Value: voteAvg})
	}
	if q.VoteCountGte > 0 {
		match = append(match, bson.E{Key: "vote_count", Value: bson.D{{Key: "$gte", Value: q.VoteCountGte}}})
	}
	if q.PopularityGte > 0 {
		match = append(match, bson.E{Key: "popularity", Value: bson.D{{Key: "$gte", Value: q.PopularityGte}}})
	}
	if q.WithOriginalLanguage != "" {
		match = append(match, bson.E{Key: "original_language", Value: q.WithOriginalLanguage})
	}
	if q.RuntimeGte > 0 || q.RuntimeLte > 0 {
		runtime := bson.D{}
		if q.RuntimeGte > 0 {
			runtime = append(runtime, bson.E{Key: "$gte", Value: q.RuntimeGte})
		}
		if q.RuntimeLte > 0 {
			runtime = append(runtime, bson.E{Key: "$lte", Value: q.RuntimeLte})
		}
		match = append(match, bson.E{Key: "runtime", Value: runtime})
	}
	if q.Softcore != nil {
		match = append(match, bson.E{Key: "softcore", Value: *q.Softcore})
	}
	return match
}

// normalizeDateGte trims RFC3339 to YYYY-MM-DD for gte comparisons.
// Both "YYYY-MM-DD" and "YYYY-MM-DDTHH:MM:SSZ" stored values sort after this prefix.
func normalizeDateGte(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// normalizeDateLte appends "Z" to a date-only string so that all RFC3339 timestamps
// on that day pass the lte check ("T" < "Z" in ASCII).
func normalizeDateLte(s string) string {
	if len(s) > 10 {
		return s[:10] + "Z"
	}
	return s + "Z"
}

// effectiveReleaseDateExpression prefers the Thailand release date when available.
func effectiveReleaseDateExpression() bson.D {
	return bson.D{{Key: "$cond", Value: bson.A{
		bson.D{{Key: "$gt", Value: bson.A{"$release_date_th", ""}}},
		"$release_date_th",
		"$release_date",
	}}}
}

func effectiveReleaseDateStage() bson.D {
	return bson.D{{Key: "$addFields", Value: bson.D{
		{Key: "_effective_release_date", Value: effectiveReleaseDateExpression()},
	}}}
}

func watchProviderStages(_ string, providers []int64) bson.A {
	if len(providers) == 0 {
		return nil
	}
	return bson.A{bson.D{{Key: "$match", Value: bson.M{
		"watch_providers": bson.M{"$in": providers},
	}}}}
}

// extractProviderIDs returns a deduplicated slice of provider IDs from a WatchProviderCountry.
func extractProviderIDs(c *WatchProviderCountry) []int64 {
	if c == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	var ids []int64
	for _, list := range [][]Flatrate{c.Flatrate, c.Rent, c.Buy, c.Ads, c.Free} {
		for _, f := range list {
			if _, ok := seen[f.ProviderID]; !ok {
				seen[f.ProviderID] = struct{}{}
				ids = append(ids, f.ProviderID)
			}
		}
	}
	return ids
}

func discoverFacet(skip, pageSize int) bson.D {
	return bson.D{{Key: "$facet", Value: bson.D{
		{Key: "metadata", Value: bson.A{
			bson.D{{Key: "$count", Value: "total"}},
		}},
		{Key: "data", Value: bson.A{
			bson.D{{Key: "$skip", Value: skip}},
			bson.D{{Key: "$limit", Value: pageSize}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "id", Value: 1}}}},
		}},
	}}}
}

func sortStage(sortBy string) bson.D {
	switch strings.ToLower(sortBy) {
	case "popularity.asc":
		return bson.D{{Key: "popularity", Value: 1}}
	case "release_date.desc":
		return bson.D{{Key: "_effective_release_date", Value: -1}}
	case "release_date.asc":
		return bson.D{{Key: "_effective_release_date", Value: 1}}
	case "vote_average.desc":
		return bson.D{{Key: "vote_average", Value: -1}}
	case "vote_average.asc":
		return bson.D{{Key: "vote_average", Value: 1}}
	case "vote_count.desc":
		return bson.D{{Key: "vote_count", Value: -1}}
	case "vote_count.asc":
		return bson.D{{Key: "vote_count", Value: 1}}
	case "title.asc":
		return bson.D{{Key: "title", Value: 1}}
	case "title.desc":
		return bson.D{{Key: "title", Value: -1}}
	case "watched_at.desc":
		return bson.D{{Key: "watched_at", Value: -1}}
	case "watched_at.asc":
		return bson.D{{Key: "watched_at", Value: 1}}
	case "watchlisted_at.desc":
		return bson.D{{Key: "watchlisted_at", Value: -1}}
	case "watchlisted_at.asc":
		return bson.D{{Key: "watchlisted_at", Value: 1}}
	default: // popularity.desc
		return bson.D{{Key: "popularity", Value: -1}}
	}
}

func sortByReleaseDate(sortBy string) bool {
	switch strings.ToLower(sortBy) {
	case "release_date.asc", "release_date.desc":
		return true
	default:
		return false
	}
}
