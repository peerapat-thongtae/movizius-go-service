package movie

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MovieRepository is the data access contract for the movie collections.
type MovieRepository interface {
	FindByUserID(ctx context.Context, userID string) ([]MovieUser, error)
	DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) (ids []int64, total int, err error)
	UpsertState(ctx context.Context, userID string, req UpsertStateRequest) error
}

type mongoMovieRepository struct {
	db *mongo.Database
}

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

func (r *mongoMovieRepository) UpsertState(ctx context.Context, userID string, req UpsertStateRequest) error {
	now := time.Now().UTC()
	filter := bson.M{"id": req.ID, "user_id": userID}

	var update bson.M
	if req.Status == "watched" {
		update = bson.M{
			"$set":         bson.M{"watched_at": now, "updated_at": now},
			"$setOnInsert": bson.M{"watchlisted_at": now, "media_type": "movie"},
		}
	} else {
		update = bson.M{
			"$set":         bson.M{"updated_at": now},
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

// DiscoverIDs returns a paginated list of TMDB movie IDs matching the query,
// along with the total count of matching documents.
func (r *mongoMovieRepository) DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) ([]int64, int, error) {
	// When filtering by account_status, start from movie_user (small, user-specific set)
	// and join into movie. This avoids scanning the full movie collection.
	var (
		coll     *mongo.Collection
		pipeline bson.A
	)
	if (len(q.WithAccountStatus) > 0 || len(q.WithoutAccountStatus) > 0) && userID != "" {
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

	// Left join movie_user when sorting by user-specific fields (watched_at, watchlisted_at).
	if sortByUserField(q.SortBy) && userID != "" {
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
		match = append(match, bson.E{Key: "genres.id", Value: bson.D{{Key: "$all", Value: q.WithGenres}}})
	}
	if len(q.WithoutGenres) > 0 {
		match = append(match, bson.E{Key: "genres.id", Value: bson.D{{Key: "$nin", Value: q.WithoutGenres}}})
	}
	if q.PrimaryReleaseYear > 0 {
		y := fmt.Sprintf("%04d", q.PrimaryReleaseYear)
		match = append(match, bson.E{Key: "release_date", Value: bson.D{
			{Key: "$gte", Value: y + "-01-01"},
			{Key: "$lte", Value: y + "-12-31"},
		}})
	} else {
		dateRange := bson.D{}
		if q.ReleaseDateGte != "" {
			dateRange = append(dateRange, bson.E{Key: "$gte", Value: q.ReleaseDateGte})
		}
		if q.ReleaseDateLte != "" {
			dateRange = append(dateRange, bson.E{Key: "$lte", Value: q.ReleaseDateLte})
		}
		if len(dateRange) > 0 {
			match = append(match, bson.E{Key: "release_date", Value: dateRange})
		}
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
	if q.WithOriginalLanguage != "" {
		match = append(match, bson.E{Key: "original_language", Value: q.WithOriginalLanguage})
	}
	if q.Softcore != nil {
		match = append(match, bson.E{Key: "softcore", Value: *q.Softcore})
	}
	return match
}

func watchProviderStages(region string, providers []int64) bson.A {
	if region == "" || len(providers) == 0 {
		return nil
	}
	field := func(t string) string {
		return fmt.Sprintf("watch_providers.%s.%s.provider_id", region, t)
	}
	inClause := bson.D{{Key: "$in", Value: providers}}
	return bson.A{bson.D{{Key: "$match", Value: bson.D{
		{Key: "$or", Value: bson.A{
			bson.D{{Key: field("flatrate"), Value: inClause}},
			bson.D{{Key: field("rent"), Value: inClause}},
			bson.D{{Key: field("buy"), Value: inClause}},
		}},
	}}}}
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
		return bson.D{{Key: "release_date", Value: -1}}
	case "release_date.asc":
		return bson.D{{Key: "release_date", Value: 1}}
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
