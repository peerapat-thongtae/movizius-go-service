package movie

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MovieRepository is the data access contract for the movie_user collection.
type MovieRepository interface {
	FindByUserID(ctx context.Context, userID string) ([]MovieUser, error)
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
