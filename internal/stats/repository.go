package stats

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Repository is the data access contract for computing watch-history stats.
type Repository interface {
	// FindWatchedMovies returns one record per movie the user watched with
	// watched_at in [from, to). Both bounds nil means unbounded.
	FindWatchedMovies(ctx context.Context, userID string, from, to *time.Time) ([]movieWatchRecord, error)
	// FindWatchedTV returns one record per TV show with at least one episode
	// watched in [from, to), with EpisodesInPeriod counting only episodes in
	// that range. Both bounds nil means unbounded.
	FindWatchedTV(ctx context.Context, userID string, from, to *time.Time) ([]tvWatchRecord, error)
}

type mongoRepository struct {
	db *mongo.Database
}

// NewRepository constructs a Repository backed by MongoDB.
func NewRepository(db *mongo.Database) Repository {
	return &mongoRepository{db: db}
}

func watchedAtMatch(from, to *time.Time) bson.M {
	m := bson.M{"$exists": true, "$ne": nil}
	if from != nil {
		m["$gte"] = *from
	}
	if to != nil {
		m["$lt"] = *to
	}
	return m
}

func (r *mongoRepository) FindWatchedMovies(ctx context.Context, userID string, from, to *time.Time) ([]movieWatchRecord, error) {
	pipeline := bson.A{
		bson.M{"$match": bson.M{
			"user_id":    userID,
			"watched_at": watchedAtMatch(from, to),
		}},
		bson.M{"$lookup": bson.M{
			"from":         "movie",
			"localField":   "id",
			"foreignField": "id",
			"as":           "movie",
		}},
		bson.M{"$unwind": "$movie"},
		bson.M{"$project": bson.M{
			"rating":            1,
			"vote_average":      "$movie.vote_average",
			"runtime":           "$movie.runtime",
			"genres":            "$movie.genres",
			"cast_ids":          "$movie.cast_ids",
			"director_id":       "$movie.director_id",
			"original_language": "$movie.original_language",
		}},
	}

	cursor, err := r.db.Collection("movie_user").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("stats: aggregate watched movies: %w", err)
	}
	defer cursor.Close(ctx)

	results := []movieWatchRecord{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("stats: decode watched movies: %w", err)
	}
	return results, nil
}

func (r *mongoRepository) FindWatchedTV(ctx context.Context, userID string, from, to *time.Time) ([]tvWatchRecord, error) {
	pipeline := bson.A{
		bson.M{"$match": bson.M{"user_id": userID}},
		bson.M{"$unwind": "$episode_watched"},
		bson.M{"$match": bson.M{
			"episode_watched.watched_at": watchedAtMatch(from, to),
		}},
		bson.M{"$group": bson.M{
			"_id":              "$id",
			"rating":           bson.M{"$first": "$rating"},
			"episodesInPeriod": bson.M{"$sum": 1},
		}},
		bson.M{"$lookup": bson.M{
			"from":         "tv",
			"localField":   "_id",
			"foreignField": "id",
			"as":           "tv",
		}},
		bson.M{"$unwind": "$tv"},
		bson.M{"$project": bson.M{
			"rating":             1,
			"vote_average":       "$tv.vote_average",
			"episode_run_time":   "$tv.episode_run_time",
			"genres":             "$tv.genres",
			"cast_ids":           "$tv.cast_ids",
			"creator_ids":        "$tv.creator_ids",
			"original_language":  "$tv.original_language",
			"episodes_in_period": "$episodesInPeriod",
		}},
	}

	cursor, err := r.db.Collection("tv_user").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("stats: aggregate watched tv: %w", err)
	}
	defer cursor.Close(ctx)

	results := []tvWatchRecord{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("stats: decode watched tv: %w", err)
	}
	return results, nil
}
