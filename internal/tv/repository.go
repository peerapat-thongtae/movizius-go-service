package tv

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TVRepository is the data access contract for the tv_user collection.
type TVRepository interface {
	FindByUserID(ctx context.Context, userID string) ([]TVUser, error)
}

type mongoTVRepository struct {
	db *mongo.Database
}

// NewRepository constructs a TVRepository backed by MongoDB.
func NewRepository(db *mongo.Database) TVRepository {
	return &mongoTVRepository{db: db}
}

func (r *mongoTVRepository) FindByUserID(ctx context.Context, userID string) ([]TVUser, error) {
	coll := r.db.Collection("tv_user")

	cursor, err := coll.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, fmt.Errorf("tv: find by user_id: %w", err)
	}
	defer cursor.Close(ctx)

	results := []TVUser{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("tv: decode results: %w", err)
	}
	return results, nil
}
