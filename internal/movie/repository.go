package movie

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MovieRepository is the data access contract for the movie collections.
type MovieRepository interface {
	FindByUserID(ctx context.Context, userID string) ([]MovieUser, error)
	DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) (ids []int64, total int, err error)
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

// DiscoverIDs returns a paginated list of TMDB movie IDs matching the query,
// along with the total count of matching documents.
func (r *mongoMovieRepository) DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) ([]int64, int, error) {
	pipeline := buildDiscoverPipeline(userID, q)
	cursor, err := r.db.Collection("movie").Aggregate(ctx, pipeline)
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

func buildDiscoverPipeline(userID string, q DiscoverQuery) bson.A {
	const pageSize = 20
	skip := (q.Page - 1) * pageSize

	pipeline := bson.A{}

	// Stage 1: initial match on the movie collection.
	match := bson.D{}
	if !q.IncludeAdult {
		// $ne: true matches both missing-field docs and explicit false — avoids
		// excluding cached docs that were stored before the adult field was set.
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
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: match}})

	// Stage 2 (optional): join movie_user to filter by account_status.
	if q.WithAccountStatus != "" && userID != "" {
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
			bson.D{{Key: "$unwind", Value: "$_user"}},
			bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "_account_status", Value: bson.D{
					{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$ifNull", Value: bson.A{"$_user.watched_at", false}}},
						"watched",
						"watchlist",
					}},
				}},
			}}},
			bson.D{{Key: "$match", Value: bson.D{{Key: "_account_status", Value: q.WithAccountStatus}}}},
		)
	}

	// Stage 3: sort.
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortStage(q.SortBy)}})

	// Stage 4 (optional): watch provider filter.
	if q.WatchRegion != "" && len(q.WithWatchProviders) > 0 {
		providerField := func(t string) string {
			return fmt.Sprintf("watch_providers.%s.%s.provider_id", q.WatchRegion, t)
		}
		inClause := bson.D{{Key: "$in", Value: q.WithWatchProviders}}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "$or", Value: bson.A{
				bson.D{{Key: providerField("flatrate"), Value: inClause}},
				bson.D{{Key: providerField("rent"), Value: inClause}},
				bson.D{{Key: providerField("buy"), Value: inClause}},
			}},
		}}})
	}

	// Stage 5: facet — count + paginated IDs.
	pipeline = append(pipeline, bson.D{{Key: "$facet", Value: bson.D{
		{Key: "metadata", Value: bson.A{
			bson.D{{Key: "$count", Value: "total"}},
		}},
		{Key: "data", Value: bson.A{
			bson.D{{Key: "$skip", Value: skip}},
			bson.D{{Key: "$limit", Value: pageSize}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "id", Value: 1}}}},
		}},
	}}})

	return pipeline
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
	default: // popularity.desc
		return bson.D{{Key: "popularity", Value: -1}}
	}
}
