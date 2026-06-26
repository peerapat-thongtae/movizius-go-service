package tv

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TVRepository is the data access contract for the tv_user collection.
type TVRepository interface {
	GetStatesByUserID(ctx context.Context, userID string) ([]TVState, error)
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
