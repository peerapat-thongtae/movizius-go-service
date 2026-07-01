package tv

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/peera/movizius-go-service/internal/shared/bsonutil"
)

// TVRepository is the data access contract for the tv_user collection.
type TVRepository interface {
	GetStatesByUserID(ctx context.Context, userID string) ([]TVStateResponse, error)
	FindByTMDBIDs(ctx context.Context, ids []int64) (map[int64]TV, error)
	DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) (ids []int64, total int, err error)
	RandomIDs(ctx context.Context, userID string, upcomingOnly bool, limit int, withoutStatus []string) (ids []int64, err error)
	UpsertTVState(ctx context.Context, userID string, tvID int64, episodes []EpisodeWatched) error
	UpsertEpisodes(ctx context.Context, userID string, req UpsertEpisodesRequest) error
	UpsertDetail(ctx context.Context, data TVResponse) error
	DeleteByTMDBID(ctx context.Context, id int64) error
	UpdateNextEpisodeAirDates(ctx context.Context, updates []NextEpisodeAirDateUpdate) error
	GetNextEpisodeAirDatesByIDs(ctx context.Context, ids []int64) (map[int64]string, error)
}

type mongoTVRepository struct {
	db *mongo.Database
}

// NewRepository constructs a TVRepository backed by MongoDB.
func NewRepository(db *mongo.Database) TVRepository {
	return &mongoTVRepository{db: db}
}

func (r *mongoTVRepository) FindByTMDBIDs(ctx context.Context, ids []int64) (map[int64]TV, error) {
	cursor, err := r.db.Collection("tv").Find(ctx, bson.M{"id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("tv: find by tmdb ids: %w", err)
	}
	defer cursor.Close(ctx)

	result := make(map[int64]TV, len(ids))
	for cursor.Next(ctx) {
		var t TV
		if err := cursor.Decode(&t); err != nil {
			return nil, fmt.Errorf("tv: decode tv: %w", err)
		}
		result[t.TVID] = t
	}
	return result, cursor.Err()
}

func (r *mongoTVRepository) UpsertTVState(ctx context.Context, userID string, tvID int64, episodes []EpisodeWatched) error {
	now := time.Now().UTC()
	filter := bson.M{"id": tvID, "user_id": userID}

	set := bson.M{"updated_at": now}
	if episodes != nil {
		set["episode_watched"] = episodes
	}

	update := bson.M{
		"$set":         set,
		"$setOnInsert": bson.M{"watchlisted_at": now, "media_type": "tv"},
	}

	_, err := r.db.Collection("tv_user").UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("tv: upsert state: %w", err)
	}
	return nil
}

func (r *mongoTVRepository) UpsertEpisodes(ctx context.Context, userID string, req UpsertEpisodesRequest) error {
	now := time.Now().UTC()
	coll := r.db.Collection("tv_user")

	// Ensure the document exists before pushing episodes.
	initFilter := bson.M{"id": req.ID, "user_id": userID}
	initUpdate := bson.M{
		"$set":         bson.M{"updated_at": now},
		"$setOnInsert": bson.M{"watchlisted_at": now, "media_type": "tv", "episode_watched": bson.A{}},
	}
	if _, err := coll.UpdateOne(ctx, initFilter, initUpdate, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("tv: upsert episodes init: %w", err)
	}

	for _, ep := range req.Episodes {
		epFilter := bson.M{
			"id":      req.ID,
			"user_id": userID,
			"episode_watched": bson.M{"$not": bson.M{"$elemMatch": bson.M{
				"season_number":  ep.SeasonNumber,
				"episode_number": ep.EpisodeNumber,
			}}},
		}
		epUpdate := bson.M{"$push": bson.M{"episode_watched": EpisodeWatched{
			EpisodeID:     ep.EpisodeID,
			SeasonNumber:  ep.SeasonNumber,
			EpisodeNumber: ep.EpisodeNumber,
			WatchedAt:     now,
		}}}
		if _, err := coll.UpdateOne(ctx, epFilter, epUpdate); err != nil {
			return fmt.Errorf("tv: upsert episodes push s%de%d: %w", ep.SeasonNumber, ep.EpisodeNumber, err)
		}
	}
	return nil
}

func (r *mongoTVRepository) DeleteByTMDBID(ctx context.Context, id int64) error {
	filter := bson.M{"id": id}
	if _, err := r.db.Collection("tv").DeleteOne(ctx, filter); err != nil {
		return fmt.Errorf("tv: delete tv %d: %w", id, err)
	}
	if _, err := r.db.Collection("tv_user").DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("tv: delete tv_user %d: %w", id, err)
	}
	return nil
}

func (r *mongoTVRepository) UpdateNextEpisodeAirDates(ctx context.Context, updates []NextEpisodeAirDateUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	now := time.Now().UTC()
	models := make([]mongo.WriteModel, 0, len(updates))
	for _, u := range updates {
		if u.ImdbID == "" {
			continue
		}
		filter := bson.M{
			"imdb_id":                            u.ImdbID,
			"next_episode_to_air.season_number":  u.SeasonNumber,
			"next_episode_to_air.episode_number": u.EpisodeNumber,
		}
		update := bson.M{"$set": bson.M{"next_episode_to_air.air_date": u.AirDate, "updated_at": now}}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(false))
	}
	if len(models) == 0 {
		return nil
	}
	if _, err := r.db.Collection("tv").BulkWrite(ctx, models); err != nil {
		return fmt.Errorf("tv: bulk update next_episode air_date: %w", err)
	}
	return nil
}

func (r *mongoTVRepository) GetNextEpisodeAirDatesByIDs(ctx context.Context, ids []int64) (map[int64]string, error) {
	cursor, err := r.db.Collection("tv").Find(ctx,
		bson.M{"id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"_id": 0, "id": 1, "next_episode_to_air.air_date": 1}),
	)
	if err != nil {
		return nil, fmt.Errorf("tv: get next episode air dates: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []struct {
		ID               int64 `bson:"id"`
		NextEpisodeToAir *struct {
			AirDate string `bson:"air_date"`
		} `bson:"next_episode_to_air"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("tv: decode next episode air dates: %w", err)
	}

	result := make(map[int64]string, len(docs))
	for _, d := range docs {
		if d.NextEpisodeToAir != nil && d.NextEpisodeToAir.AirDate != "" {
			result[d.ID] = d.NextEpisodeToAir.AirDate
		}
	}
	return result, nil
}

func (r *mongoTVRepository) UpsertDetail(ctx context.Context, data TVResponse) error {
	now := time.Now().UTC()
	filter := bson.M{"id": data.ID}
	model := tvToModel(data, now)
	update := bson.M{
		"$set": bsonutil.StructToBsonM(model, "_id", "id", "vote_average", "vote_count"),
		// vote_average and vote_count are owned by IMDB sync; only set on first insert.
		"$setOnInsert": bson.M{
			"vote_average": data.VoteAverage,
			"vote_count":   data.VoteCount,
		},
	}
	_, err := r.db.Collection("tv").UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("tv: upsert detail: %w", err)
	}
	return nil
}

func (r *mongoTVRepository) GetStatesByUserID(ctx context.Context, userID string) ([]TVStateResponse, error) {
	pipeline := buildStatesPipeline(userID)

	cursor, err := r.db.Collection("tv_user").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("tv: aggregate states: %w", err)
	}
	defer cursor.Close(ctx)

	results := []TVStateResponse{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("tv: decode states: %w", err)
	}
	return results, nil
}

// DiscoverIDs returns a paginated list of TMDB TV IDs matching the query,
// along with the total count of matching documents.
func (r *mongoTVRepository) DiscoverIDs(ctx context.Context, userID string, q DiscoverQuery) ([]int64, int, error) {
	// When filtering by account_status, start from tv_user (small, user-specific set)
	// and join into tv. This avoids scanning the full tv collection.
	var (
		coll     *mongo.Collection
		pipeline bson.A
	)
	if (len(q.WithAccountStatus) > 0 || len(q.WithoutAccountStatus) > 0) && userID != "" {
		coll = r.db.Collection("tv_user")
		pipeline = buildAccountStatusPipeline(userID, q)
	} else {
		coll = r.db.Collection("tv")
		pipeline = buildDiscoverPipeline(userID, q)
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
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

// RandomIDs returns up to limit random TMDB TV IDs from the tv collection, excluding
// any series whose derived account_status (watchlist/watching/waiting_next_ep/watched,
// same derivation as GetStates) is in withoutStatus. Series with no tv_user record are
// always eligible. When upcomingOnly is true, only series with a future
// next_episode_to_air.air_date (falling back to first_air_date when absent) qualify.
// Otherwise the pool is narrowed to the top 100 by popularity before sampling.
func (r *mongoTVRepository) RandomIDs(ctx context.Context, userID string, upcomingOnly bool, limit int, withoutStatus []string) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}

	today := time.Now().UTC().Format("2006-01-02")

	match := bson.D{{Key: "adult", Value: bson.D{{Key: "$ne", Value: true}}}}
	if upcomingOnly {
		effectiveAirDate := bson.D{{Key: "$ifNull", Value: bson.A{"$next_episode_to_air.air_date", "$first_air_date"}}}
		match = append(match, bson.E{Key: "$expr", Value: bson.D{{Key: "$gte", Value: bson.A{effectiveAirDate, today}}}})
	}

	pipeline := bson.A{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "tv_user"},
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
	}

	if len(withoutStatus) > 0 {
		pipeline = append(pipeline,
			bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "_count_watched", Value: bson.D{{Key: "$size", Value: bson.D{
					{Key: "$ifNull", Value: bson.A{"$_user.episode_watched", bson.A{}}},
				}}}},
				{Key: "_max_ep", Value: maxEpReduceExpr("$_user.episode_watched")},
			}}},
			bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "_account_status", Value: bson.D{{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$eq", Value: bson.A{"$_user", nil}}},
					nil,
					bson.D{{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$and", Value: bson.A{
							bson.D{{Key: "$gt", Value: bson.A{"$number_of_episodes", 0}}},
							bson.D{{Key: "$eq", Value: bson.A{"$_count_watched", "$number_of_episodes"}}},
							bson.D{{Key: "$ne", Value: bson.A{"$status", "Returning Series"}}},
						}}},
						"watched",
						bson.D{{Key: "$cond", Value: bson.A{
							bson.D{{Key: "$and", Value: bson.A{
								bson.D{{Key: "$gt", Value: bson.A{"$_count_watched", 0}}},
								bson.D{{Key: "$eq", Value: bson.A{"$_max_ep.season_number", "$last_episode_to_air.season_number"}}},
								bson.D{{Key: "$eq", Value: bson.A{"$_max_ep.episode_number", "$last_episode_to_air.episode_number"}}},
								bson.D{{Key: "$or", Value: bson.A{
									bson.D{{Key: "$eq", Value: bson.A{"$next_episode_to_air", nil}}},
									bson.D{{Key: "$gt", Value: bson.A{"$next_episode_to_air.air_date", today}}},
								}}},
							}}},
							"waiting_next_ep",
							bson.D{{Key: "$cond", Value: bson.A{
								bson.D{{Key: "$gt", Value: bson.A{"$_count_watched", 0}}},
								"watching",
								"watchlist",
							}}},
						}}},
					}}},
				}}}},
			}}},
			bson.D{{Key: "$match", Value: bson.D{{Key: "_account_status", Value: bson.D{{Key: "$nin", Value: withoutStatus}}}}}},
		)
	}

	if !upcomingOnly {
		pipeline = append(pipeline,
			bson.D{{Key: "$sort", Value: bson.D{{Key: "popularity", Value: -1}}}},
			bson.D{{Key: "$limit", Value: 100}},
		)
	}

	pipeline = append(pipeline,
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: limit}}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "id", Value: 1}}}},
	)

	cursor, err := r.db.Collection("tv").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("tv: random aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []struct {
		ID int64 `bson:"id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("tv: decode random: %w", err)
	}

	ids := make([]int64, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// buildDiscoverPipeline runs on the tv collection (no account_status filter).
// When sort_by=max_watched_ep.* it still needs a user join, but as a left join
// so all shows are retained (un-watched shows get _max_watched_at=null).
func buildDiscoverPipeline(userID string, q DiscoverQuery) bson.A {
	const pageSize = 20
	skip := (q.Page - 1) * pageSize

	pipeline := bson.A{}
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: tvMatchConditions(q)}})

	sortByProgress := strings.HasPrefix(strings.ToLower(q.SortBy), "max_watched_ep")
	if sortByProgress && userID != "" {
		pipeline = append(pipeline, bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "tv_user"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},
			}},
			{Key: "as", Value: "_user"},
		}}})
		pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$_user"},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}})
		pipeline = append(pipeline, bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "_max_watched_at", Value: bson.D{{Key: "$max", Value: bson.D{
				{Key: "$ifNull", Value: bson.A{"$_user.episode_watched.watched_at", bson.A{}}},
			}}}},
		}}})
	}

	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: tvSortStage(q.SortBy)}})
	pipeline = append(pipeline, tvWatchProviderStages(q.WatchRegion, q.WithWatchProviders)...)
	pipeline = append(pipeline, discoverFacet(skip, pageSize))
	return pipeline
}

// buildAccountStatusPipeline runs on tv_user so it starts from a small,
// user-scoped set. It joins tv only for the filtered user entries.
func buildAccountStatusPipeline(userID string, q DiscoverQuery) bson.A {
	today := time.Now().Format("2006-01-02")
	const pageSize = 20
	skip := (q.Page - 1) * pageSize

	pipeline := bson.A{
		// 1. Small initial set: only this user's TV entries.
		bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}}}},

		// 2. Compute progress fields from episode_watched.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "_count_watched", Value: bson.D{{Key: "$size", Value: bson.D{
				{Key: "$ifNull", Value: bson.A{"$episode_watched", bson.A{}}},
			}}}},
			{Key: "_max_ep", Value: maxEpReduceExpr("$episode_watched")},
			{Key: "_max_watched_at", Value: bson.D{{Key: "$max", Value: "$episode_watched.watched_at"}}},
		}}},

		// 3. Join tv to get series metadata needed for account_status derivation.
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "tv"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "as", Value: "_tv"},
		}}},
		bson.D{{Key: "$unwind", Value: "$_tv"}},

		// 4. Derive account_status using joined tv fields.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "_account_status", Value: bson.D{
				{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$and", Value: bson.A{
						bson.D{{Key: "$gt", Value: bson.A{"$_tv.number_of_episodes", 0}}},
						bson.D{{Key: "$eq", Value: bson.A{"$_count_watched", "$_tv.number_of_episodes"}}},
						bson.D{{Key: "$ne", Value: bson.A{"$_tv.status", "Returning Series"}}},
					}}},
					"watched",
					bson.D{{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$and", Value: bson.A{
							bson.D{{Key: "$gt", Value: bson.A{"$_count_watched", 0}}},
							bson.D{{Key: "$eq", Value: bson.A{"$_max_ep.season_number", "$_tv.last_episode_to_air.season_number"}}},
							bson.D{{Key: "$eq", Value: bson.A{"$_max_ep.episode_number", "$_tv.last_episode_to_air.episode_number"}}},
							bson.D{{Key: "$or", Value: bson.A{
								bson.D{{Key: "$eq", Value: bson.A{"$_tv.next_episode_to_air", nil}}},
								bson.D{{Key: "$gt", Value: bson.A{"$_tv.next_episode_to_air.air_date", today}}},
							}}},
						}}},
						"waiting_next_ep",
						bson.D{{Key: "$cond", Value: bson.A{
							bson.D{{Key: "$gt", Value: bson.A{"$_count_watched", 0}}},
							"watching",
							"watchlist",
						}}},
					}}},
				}},
			}},
		}}},

		// 5. Filter to requested status before promoting tv fields.
		bson.D{{Key: "$match", Value: accountStatusMatchCond(q.WithAccountStatus, q.WithoutAccountStatus)}},

		// 6. Promote tv fields to root; preserve user-derived fields.
		bson.D{{Key: "$replaceRoot", Value: bson.D{
			{Key: "newRoot", Value: bson.D{{Key: "$mergeObjects", Value: bson.A{
				"$_tv",
				bson.D{
					{Key: "_account_status", Value: "$_account_status"},
					{Key: "_max_watched_at", Value: "$_max_watched_at"},
					{Key: "_count_watched", Value: "$_count_watched"},
					{Key: "_max_ep", Value: "$_max_ep"},
				},
			}}}},
		}}},
	}

	// 7. Optional tv filters (applied after merge so field names match tv schema).
	if cond := tvMatchConditions(q); len(cond) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: cond}})
	}

	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: tvSortStage(q.SortBy)}})
	pipeline = append(pipeline, tvWatchProviderStages(q.WatchRegion, q.WithWatchProviders)...)
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

// tvMatchConditions builds $match conditions for fields on the tv collection.
func tvMatchConditions(q DiscoverQuery) bson.D {
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
	if q.NextEpisodeAirDateGte != "" || q.NextEpisodeAirDateLte != "" {
		// Normalize both the stored field and the input params to full RFC3339 before
		// comparing, because the DB may store either "YYYY-MM-DD" or "YYYY-MM-DDT...Z".
		// $ifNull guards against missing/null next_episode_to_air.air_date.
		safeField := bson.D{{Key: "$ifNull", Value: bson.A{"$next_episode_to_air.air_date", ""}}}
		normalizedField := bson.D{{Key: "$cond", Value: bson.A{
			bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$strLenCP", Value: safeField}}, 10}}},
			bson.D{{Key: "$concat", Value: bson.A{"$next_episode_to_air.air_date", "T00:00:00Z"}}},
			safeField,
		}}}

		var exprs bson.A
		if q.NextEpisodeAirDateGte != "" {
			v := q.NextEpisodeAirDateGte
			if len(v) == 10 {
				v += "T00:00:00Z"
			}
			exprs = append(exprs, bson.D{{Key: "$gte", Value: bson.A{normalizedField, v}}})
		}
		if q.NextEpisodeAirDateLte != "" {
			v := q.NextEpisodeAirDateLte
			if len(v) == 10 {
				v += "T23:59:00Z"
			}
			exprs = append(exprs, bson.D{{Key: "$lte", Value: bson.A{normalizedField, v}}})
		}

		var exprCond any
		if len(exprs) == 1 {
			exprCond = exprs[0]
		} else {
			exprCond = bson.D{{Key: "$and", Value: exprs}}
		}
		match = append(match, bson.E{Key: "$expr", Value: exprCond})
	}
	return match
}

// maxEpReduceExpr returns the $reduce expression that finds the episode with the
// highest (season_number, episode_number) from the given array field path.
func maxEpReduceExpr(arrayField string) bson.D {
	return bson.D{
		{Key: "$reduce", Value: bson.D{
			{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{arrayField, bson.A{}}}}},
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
}

func tvWatchProviderStages(_ string, providers []int64) bson.A {
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
			{Key: "max_watched_ep", Value: maxEpReduceExpr("$episode_watched")},
			{Key: "count_watched", Value: bson.D{{Key: "$size", Value: bson.D{
				{Key: "$ifNull", Value: bson.A{"$episode_watched", bson.A{}}},
			}}}},
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
					// watched: finished all episodes of a completed series (guard number_of_episodes > 0)
					{Key: "if", Value: bson.D{{Key: "$and", Value: bson.A{
						bson.D{{Key: "$gt", Value: bson.A{"$tv.number_of_episodes", 0}}},
						bson.D{{Key: "$eq", Value: bson.A{"$count_watched", "$tv.number_of_episodes"}}},
						bson.D{{Key: "$not", Value: bson.A{
							bson.D{{Key: "$eq", Value: bson.A{"$tv.status", "Returning Series"}}},
						}}},
					}}}},
					{Key: "then", Value: "watched"},
					{Key: "else", Value: bson.D{
						{Key: "$cond", Value: bson.D{
							// waiting_next_ep: caught up to latest aired ep and no new ep available yet
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

		// Compute watched_seasons: season numbers where the user has watched all episodes.
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "watched_seasons", Value: bson.D{
				{Key: "$map", Value: bson.D{
					{Key: "input", Value: bson.D{
						{Key: "$filter", Value: bson.D{
							{Key: "input", Value: "$tv.seasons"},
							{Key: "as", Value: "season"},
							{Key: "cond", Value: bson.D{{Key: "$and", Value: bson.A{
								bson.D{{Key: "$gt", Value: bson.A{"$$season.season_number", 0}}},
								bson.D{{Key: "$eq", Value: bson.A{
									"$$season.episode_count",
									bson.D{{Key: "$size", Value: bson.D{
										{Key: "$filter", Value: bson.D{
											{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$episode_watched", bson.A{}}}}},
											{Key: "as", Value: "ep"},
											{Key: "cond", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ep.season_number", "$$season.season_number"}}}},
										}},
									}}},
								}}},
							}}}},
						}},
					}},
					{Key: "as", Value: "s"},
					{Key: "in", Value: "$$s.season_number"},
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
			{Key: "watched_seasons", Value: 1},
		}}},
	}
}
