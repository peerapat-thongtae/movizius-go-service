package tv

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TVRepository is the data access contract for the tv_user collection.
type TVRepository interface {
	GetStatesByUserID(ctx context.Context, userID string) ([]TVState, error)
	DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) (ids []int64, total int, err error)
}

type mongoTVRepository struct {
	db *mongo.Database
}

// NewRepository constructs a TVRepository backed by MongoDB.
func NewRepository(db *mongo.Database) TVRepository {
	return &mongoTVRepository{db: db}
}

func (r *mongoTVRepository) GetStatesByUserID(ctx context.Context, userID string) ([]TVState, error) {
	pipeline := buildStatesPipeline(userID)

	cursor, err := r.db.Collection("tv_user").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("tv: aggregate states: %w", err)
	}
	defer cursor.Close(ctx)

	results := []TVState{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("tv: decode states: %w", err)
	}
	return results, nil
}

// DiscoverIDs returns a paginated list of TMDB TV IDs matching the query,
// along with the total count of matching documents.
func (r *mongoTVRepository) DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) ([]int64, int, error) {
	pipeline := buildDiscoverPipeline(userID, q)
	cursor, err := r.db.Collection("tv").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, fmt.Errorf("tv: discover aggregate: %w", err)
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
		return nil, 0, fmt.Errorf("tv: decode discover: %w", err)
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

	// Stage 1: initial match on the tv collection.
	match := bson.D{}
	if !q.IncludeAdult {
		match = append(match, bson.E{Key: "adult", Value: bson.D{{Key: "$ne", Value: true}}})
	}
	if len(q.WithGenres) > 0 {
		match = append(match, bson.E{Key: "genres.id", Value: bson.D{{Key: "$all", Value: q.WithGenres}}})
	}
	if len(q.WithoutGenres) > 0 {
		match = append(match, bson.E{Key: "genres.id", Value: bson.D{{Key: "$nin", Value: q.WithoutGenres}}})
	}
	if q.FirstAirDateYear > 0 {
		y := fmt.Sprintf("%04d", q.FirstAirDateYear)
		match = append(match, bson.E{Key: "first_air_date", Value: bson.D{
			{Key: "$gte", Value: y + "-01-01"},
			{Key: "$lte", Value: y + "-12-31"},
		}})
	} else {
		dateRange := bson.D{}
		if q.FirstAirDateGte != "" {
			dateRange = append(dateRange, bson.E{Key: "$gte", Value: q.FirstAirDateGte})
		}
		if q.FirstAirDateLte != "" {
			dateRange = append(dateRange, bson.E{Key: "$lte", Value: q.FirstAirDateLte})
		}
		if len(dateRange) > 0 {
			match = append(match, bson.E{Key: "first_air_date", Value: dateRange})
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
	if len(q.WithNetworks) > 0 {
		match = append(match, bson.E{Key: "networks.id", Value: bson.D{{Key: "$in", Value: q.WithNetworks}}})
	}
	if q.IsAnime != nil {
		match = append(match, bson.E{Key: "is_anime", Value: *q.IsAnime})
	}
	if q.WithStatus != "" {
		match = append(match, bson.E{Key: "status", Value: q.WithStatus})
	}
	if q.WithType != "" {
		match = append(match, bson.E{Key: "type", Value: q.WithType})
	}
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: match}})

	// Stage 2 (optional): join tv_user to compute _max_ep / filter by account_status.
	// Triggered when filtering by status OR sorting by max_watched_ep.
	sortByProgress := strings.HasPrefix(strings.ToLower(q.SortBy), "max_watched_ep")
	needsUserJoin := userID != "" && (q.WithAccountStatus != "" || sortByProgress)

	if needsUserJoin {
		pipeline = append(pipeline, bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "tv_user"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},
			}},
			{Key: "as", Value: "_user"},
		}}})

		// Inner join when filtering by status (drops shows not in tv_user).
		// Left join when only sorting (preserves shows not in tv_user with _max_ep=null).
		if q.WithAccountStatus != "" {
			pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: "$_user"}})
		} else {
			pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: bson.D{
				{Key: "path", Value: "$_user"},
				{Key: "preserveNullAndEmptyArrays", Value: true},
			}}})
		}

		// $ifNull guards against missing _user (left-join case).
		maxEpReduce := bson.D{
			{Key: "$reduce", Value: bson.D{
				{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$_user.episode_watched", bson.A{}}}}},
				{Key: "initialValue", Value: nil},
				{Key: "in", Value: bson.D{
					{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$or", Value: bson.A{
							bson.D{{Key: "$gt", Value: bson.A{"$$this.season_number", "$$value.season_number"}}},
							bson.D{{Key: "$and", Value: bson.A{
								bson.D{{Key: "$eq", Value: bson.A{"$$this.season_number", "$$value.season_number"}}},
								bson.D{{Key: "$gt", Value: bson.A{"$$this.episode_number", "$$value.episode_number"}}},
							}}},
						}}},
						"$$this",
						"$$value",
					}},
				}},
			}},
		}
		pipeline = append(pipeline, bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "_count_watched", Value: bson.D{{Key: "$size", Value: bson.D{
				{Key: "$ifNull", Value: bson.A{"$_user.episode_watched", bson.A{}}},
			}}}},
			{Key: "_max_ep", Value: maxEpReduce},
			// Latest watched_at across all watched episodes — used for max_watched_ep sort.
			{Key: "_max_watched_at", Value: bson.D{{Key: "$max", Value: "$_user.episode_watched.watched_at"}}},
		}}})

		if q.WithAccountStatus != "" {
			pipeline = append(pipeline,
				bson.D{{Key: "$addFields", Value: bson.D{
					{Key: "_account_status", Value: bson.D{
						{Key: "$cond", Value: bson.A{
							// watched: all eps done and series is not still airing
							bson.D{{Key: "$and", Value: bson.A{
								bson.D{{Key: "$eq", Value: bson.A{"$_count_watched", "$number_of_episodes"}}},
								bson.D{{Key: "$ne", Value: bson.A{"$status", "Returning Series"}}},
							}}},
							"watched",
							bson.D{{Key: "$cond", Value: bson.A{
								// wait_next_season: caught up to last aired ep
								bson.D{{Key: "$and", Value: bson.A{
									bson.D{{Key: "$gt", Value: bson.A{"$_count_watched", 0}}},
									bson.D{{Key: "$eq", Value: bson.A{"$_max_ep.season_number", "$last_episode_to_air.season_number"}}},
									bson.D{{Key: "$eq", Value: bson.A{"$_max_ep.episode_number", "$last_episode_to_air.episode_number"}}},
								}}},
								"wait_next_season",
								bson.D{{Key: "$cond", Value: bson.A{
									bson.D{{Key: "$gt", Value: bson.A{"$_count_watched", 0}}},
									"watching",
									"watchlist",
								}}},
							}}},
						}},
					}},
				}}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "_account_status", Value: q.WithAccountStatus}}}},
			)
		}
	}

	// Stage 3: sort.
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: tvSortStage(q.SortBy)}})

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

func tvSortStage(sortBy string) bson.D {
	switch strings.ToLower(sortBy) {
	case "popularity.asc":
		return bson.D{{Key: "popularity", Value: 1}}
	case "first_air_date.desc":
		return bson.D{{Key: "first_air_date", Value: -1}}
	case "first_air_date.asc":
		return bson.D{{Key: "first_air_date", Value: 1}}
	case "vote_average.desc":
		return bson.D{{Key: "vote_average", Value: -1}}
	case "vote_average.asc":
		return bson.D{{Key: "vote_average", Value: 1}}
	case "vote_count.desc":
		return bson.D{{Key: "vote_count", Value: -1}}
	case "vote_count.asc":
		return bson.D{{Key: "vote_count", Value: 1}}
	case "name.asc":
		return bson.D{{Key: "name", Value: 1}}
	case "name.desc":
		return bson.D{{Key: "name", Value: -1}}
	case "max_watched_ep.desc":
		return bson.D{{Key: "_max_watched_at", Value: -1}}
	case "max_watched_ep.asc":
		return bson.D{{Key: "_max_watched_at", Value: 1}}
	default: // popularity.desc
		return bson.D{{Key: "popularity", Value: -1}}
	}
}

// buildStatesPipeline constructs the aggregation pipeline that joins tv_user with
// the tv collection and computes account_status and related derived fields.
// today is injected at query time to match dayjs().format('YYYY-MM-DD') behaviour.
func buildStatesPipeline(userID string) bson.A {
	today := time.Now().Format("2006-01-02")

	return bson.A{
		// Filter early before the expensive join.
		bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},

		// Compute max_watched_ep (highest season/episode) and count_watched.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "max_watched_ep", Value: bson.D{
				{Key: "$reduce", Value: bson.D{
					{Key: "input", Value: "$episode_watched"},
					{Key: "initialValue", Value: nil},
					{Key: "in", Value: bson.D{
						{Key: "$cond", Value: bson.A{
							bson.D{{Key: "$or", Value: bson.A{
								bson.D{{Key: "$gt", Value: bson.A{"$$this.season_number", "$$value.season_number"}}},
								bson.D{{Key: "$and", Value: bson.A{
									bson.D{{Key: "$eq", Value: bson.A{"$$this.season_number", "$$value.season_number"}}},
									bson.D{{Key: "$gt", Value: bson.A{"$$this.episode_number", "$$value.episode_number"}}},
								}}},
							}}},
							"$$this",
							"$$value",
						}},
					}},
				}},
			}},
			{Key: "count_watched", Value: bson.D{{Key: "$size", Value: "$episode_watched"}}},
		}}},

		// Derive latest_watched from the max episode's watched_at.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "latest_watched", Value: "$max_watched_ep.watched_at"},
		}}},

		// Join with the tv collection on the TMDB id field.
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "tv"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "as", Value: "tv"},
		}}},
		bson.D{{Key: "$unwind", Value: "$tv"}},

		// Compute account_status based on watched episode count and series state.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "account_status", Value: bson.D{
				{Key: "$cond", Value: bson.D{
					// watched: finished all episodes of a completed series
					{Key: "if", Value: bson.D{{Key: "$and", Value: bson.A{
						bson.D{{Key: "$eq", Value: bson.A{"$count_watched", "$tv.number_of_episodes"}}},
						bson.D{{Key: "$not", Value: bson.A{
							bson.D{{Key: "$eq", Value: bson.A{"$tv.status", "Returning Series"}}},
						}}},
					}}}},
					{Key: "then", Value: "watched"},
					{Key: "else", Value: bson.D{
						{Key: "$cond", Value: bson.D{
							// waiting_next_ep: caught up to latest aired ep and waiting
							{Key: "if", Value: bson.D{{Key: "$and", Value: bson.A{
								bson.D{{Key: "$gt", Value: bson.A{"$count_watched", 0}}},
								bson.D{{Key: "$eq", Value: bson.A{"$max_watched_ep.season_number", "$tv.last_episode_to_air.season_number"}}},
								bson.D{{Key: "$eq", Value: bson.A{"$max_watched_ep.episode_number", "$tv.last_episode_to_air.episode_number"}}},
								bson.D{{Key: "$or", Value: bson.A{
									bson.D{{Key: "$eq", Value: bson.A{"$tv.next_episode_to_air", nil}}},
									bson.D{{Key: "$gt", Value: bson.A{"$tv.next_episode_to_air.air_date", today}}},
								}}},
							}}}},
							{Key: "then", Value: "waiting_next_ep"},
							{Key: "else", Value: bson.D{
								{Key: "$cond", Value: bson.D{
									{Key: "if", Value: bson.D{{Key: "$gt", Value: bson.A{"$count_watched", 0}}}},
									{Key: "then", Value: "watching"},
									{Key: "else", Value: "watchlist"},
								}},
							}},
						}},
					}},
				}},
			}},
		}}},

		// latest_state: the timestamp that best represents "when was this last active"
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "latest_state", Value: bson.D{
				{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{{Key: "$eq", Value: bson.A{"$account_status", "watched"}}}},
					{Key: "then", Value: "$latest_watched"},
					{Key: "else", Value: "$watchlisted_at"},
				}},
			}},
		}}},

		// Project the final response shape, pulling joined tv fields up.
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "id", Value: 1},
			{Key: "user_id", Value: 1},
			{Key: "name", Value: "$tv.name"},
			{Key: "media_type", Value: "tv"},
			{Key: "is_anime", Value: "$tv.is_anime"},
			{Key: "vote_average", Value: "$tv.vote_average"},
			{Key: "vote_count", Value: "$tv.vote_count"},
			{Key: "number_of_episodes", Value: "$tv.number_of_episodes"},
			{Key: "number_of_seasons", Value: "$tv.number_of_seasons"},
			{Key: "episode_watched", Value: 1},
			{Key: "latest_watched", Value: 1},
			{Key: "watchlisted_at", Value: 1},
			{Key: "count_watched", Value: 1},
			{Key: "account_status", Value: 1},
			{Key: "latest_state", Value: 1},
			{Key: "max_watched_ep", Value: 1},
			{Key: "next_episode_to_air", Value: "$tv.next_episode_to_air"},
			{Key: "last_episode_to_air", Value: "$tv.last_episode_to_air"},
			{Key: "seasons", Value: "$tv.seasons"},
		}}},
	}
}
