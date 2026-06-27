package database

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsureIndexes creates all indexes required by the discover pipelines.
// It is idempotent — safe to call on every cold start.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	if err := ensureMovieIndexes(ctx, db); err != nil {
		return fmt.Errorf("ensure movie indexes: %w", err)
	}
	if err := ensureTVIndexes(ctx, db); err != nil {
		return fmt.Errorf("ensure tv indexes: %w", err)
	}
	if err := ensureMovieUserIndexes(ctx, db); err != nil {
		return fmt.Errorf("ensure movie_user indexes: %w", err)
	}
	if err := ensureTVUserIndexes(ctx, db); err != nil {
		return fmt.Errorf("ensure tv_user indexes: %w", err)
	}
	return nil
}

func idx(key bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: key, Options: options.Index().SetBackground(false)}
}

func ensureMovieIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("movie").Indexes().CreateMany(ctx, []mongo.IndexModel{
		idx(bson.D{{Key: "id", Value: 1}}), // lookup target from movie_user
		idx(bson.D{{Key: "popularity", Value: -1}}),
		idx(bson.D{{Key: "release_date", Value: -1}}),
		idx(bson.D{{Key: "vote_average", Value: -1}}),
		idx(bson.D{{Key: "vote_count", Value: -1}}),
		idx(bson.D{{Key: "title", Value: 1}}),
		idx(bson.D{{Key: "genres.id", Value: 1}}),
		idx(bson.D{{Key: "original_language", Value: 1}}),
	})
	return err
}

func ensureTVIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("tv").Indexes().CreateMany(ctx, []mongo.IndexModel{
		idx(bson.D{{Key: "id", Value: 1}}), // lookup target from tv_user
		idx(bson.D{{Key: "popularity", Value: -1}}),
		idx(bson.D{{Key: "first_air_date", Value: -1}}),
		idx(bson.D{{Key: "vote_average", Value: -1}}),
		idx(bson.D{{Key: "vote_count", Value: -1}}),
		idx(bson.D{{Key: "name", Value: 1}}),
		idx(bson.D{{Key: "genres.id", Value: 1}}),
		idx(bson.D{{Key: "original_language", Value: 1}}),
		idx(bson.D{{Key: "networks.id", Value: 1}}),
		idx(bson.D{{Key: "is_anime", Value: 1}}),
		idx(bson.D{{Key: "status", Value: 1}}),
	})
	return err
}

func ensureMovieUserIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("movie_user").Indexes().CreateMany(ctx, []mongo.IndexModel{
		idx(bson.D{{Key: "user_id", Value: 1}}),                       // initial match in account_status pipeline
		idx(bson.D{{Key: "id", Value: 1}, {Key: "user_id", Value: 1}}), // lookup sub-pipeline
	})
	return err
}

func ensureTVUserIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("tv_user").Indexes().CreateMany(ctx, []mongo.IndexModel{
		idx(bson.D{{Key: "user_id", Value: 1}}),                       // initial match in account_status pipeline
		idx(bson.D{{Key: "id", Value: 1}, {Key: "user_id", Value: 1}}), // lookup sub-pipeline
	})
	return err
}
