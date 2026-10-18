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
	FindWatchedTV(ctx context.Context, userID string, from, to *time.Time, isAnime *bool) ([]tvWatchRecord, error)
	GetLibrarySnapshot(ctx context.Context, userID string, isAnime *bool) (LibrarySnapshot, Completion, error)
	FindWatchedTitlesByPerson(ctx context.Context, userID string, personID int64, role, mediaType string, from, to *time.Time) ([]PersonTitle, error)
}

func (r *mongoRepository) GetLibrarySnapshot(ctx context.Context, userID string, isAnime *bool) (LibrarySnapshot, Completion, error) {
	moviesToWatch, err := r.db.Collection("movie_user").CountDocuments(ctx, bson.M{
		"user_id":    userID,
		"watched_at": nil,
	})
	if err != nil {
		return LibrarySnapshot{}, Completion{}, fmt.Errorf("stats: count movie watchlist: %w", err)
	}
	moviesCompleted, err := r.db.Collection("movie_user").CountDocuments(ctx, bson.M{
		"user_id":    userID,
		"watched_at": bson.M{"$ne": nil},
	})
	if err != nil {
		return LibrarySnapshot{}, Completion{}, fmt.Errorf("stats: count completed movies: %w", err)
	}

	pipeline := bson.A{
		bson.M{"$match": bson.M{"user_id": userID}},
		bson.M{"$lookup": bson.M{"from": "tv", "localField": "id", "foreignField": "id", "as": "tv"}},
		bson.M{"$unwind": "$tv"},
		bson.M{"$addFields": bson.M{"_watched_count": bson.M{"$size": bson.M{"$ifNull": bson.A{"$episode_watched", bson.A{}}}}}},
		bson.M{"$group": bson.M{
			"_id":      nil,
			"to_watch": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$_watched_count", 0}}, 1, 0}}},
			"completed": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$and": bson.A{
				bson.M{"$gt": bson.A{"$tv.number_of_episodes", 0}},
				bson.M{"$gte": bson.A{"$_watched_count", "$tv.number_of_episodes"}},
				bson.M{"$ne": bson.A{"$tv.status", "Returning Series"}},
			}}, 1, 0}}},
			"in_progress": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$and": bson.A{
				bson.M{"$gt": bson.A{"$_watched_count", 0}},
				bson.M{"$or": bson.A{
					bson.M{"$eq": bson.A{"$tv.number_of_episodes", 0}},
					bson.M{"$lt": bson.A{"$_watched_count", "$tv.number_of_episodes"}},
					bson.M{"$eq": bson.A{"$tv.status", "Returning Series"}},
				}},
			}}, 1, 0}}},
		}},
	}
	if isAnime != nil {
		pipeline = append(pipeline[:3], append(bson.A{bson.M{"$match": bson.M{"tv.is_anime": *isAnime}}}, pipeline[3:]...)...)
	}
	cursor, err := r.db.Collection("tv_user").Aggregate(ctx, pipeline)
	if err != nil {
		return LibrarySnapshot{}, Completion{}, fmt.Errorf("stats: aggregate tv library status: %w", err)
	}
	defer cursor.Close(ctx)
	var tvResult []struct {
		ToWatch    int `bson:"to_watch"`
		Completed  int `bson:"completed"`
		InProgress int `bson:"in_progress"`
	}
	if err := cursor.All(ctx, &tvResult); err != nil {
		return LibrarySnapshot{}, Completion{}, fmt.Errorf("stats: decode tv library status: %w", err)
	}
	snapshot := LibrarySnapshot{MoviesToWatch: int(moviesToWatch)}
	completion := Completion{Movies: int(moviesCompleted)}
	if len(tvResult) > 0 {
		snapshot.TVToWatch = tvResult[0].ToWatch
		snapshot.TVInProgress = tvResult[0].InProgress
		completion.TVShows = tvResult[0].Completed
	}
	return snapshot, completion, nil
}

func (r *mongoRepository) FindWatchedTitlesByPerson(ctx context.Context, userID string, personID int64, role, mediaType string, from, to *time.Time) ([]PersonTitle, error) {
	movieMatch := bson.M{"movie.cast_ids": personID}
	tvMatch := bson.M{"tv.cast_ids": personID}
	if role == "director" {
		movieMatch = bson.M{"movie.director_id": personID}
		tvMatch = bson.M{"tv.creator_ids": personID}
	}

	moviePipeline := bson.A{
		bson.M{"$match": bson.M{"user_id": userID, "watched_at": watchedAtMatch(from, to)}},
		bson.M{"$lookup": bson.M{"from": "movie", "localField": "id", "foreignField": "id", "as": "movie"}},
		bson.M{"$unwind": "$movie"},
		bson.M{"$match": movieMatch},
		bson.M{"$project": bson.M{"id": "$movie.id", "media_type": bson.M{"$literal": "movie"}, "title": "$movie.title", "poster_path": "$movie.poster_path", "release_date": "$movie.release_date", "watched_at": "$watched_at"}},
	}
	tvPipeline := bson.A{
		bson.M{"$match": bson.M{"user_id": userID}},
		bson.M{"$unwind": "$episode_watched"},
		bson.M{"$match": bson.M{"episode_watched.watched_at": watchedAtMatch(from, to)}},
		bson.M{"$group": bson.M{"_id": "$id", "watched_at": bson.M{"$max": "$episode_watched.watched_at"}}},
		bson.M{"$lookup": bson.M{"from": "tv", "localField": "_id", "foreignField": "id", "as": "tv"}},
		bson.M{"$unwind": "$tv"},
		bson.M{"$match": tvMatch},
		bson.M{"$project": bson.M{"id": "$tv.id", "media_type": bson.M{"$literal": "tv"}, "title": "$tv.name", "poster_path": "$tv.poster_path", "first_air_date": "$tv.first_air_date", "watched_at": 1}},
	}
	if mediaType == "tv" || mediaType == "anime" {
		isAnime := mediaType == "anime"
		tvPipeline = append(tvPipeline[:6], append(bson.A{bson.M{"$match": bson.M{"tv.is_anime": isAnime}}}, tvPipeline[6:]...)...)
	}

	results := make([]PersonTitle, 0)
	for _, query := range []struct {
		mediaType  string
		collection string
		pipeline   bson.A
	}{{"movie", "movie_user", moviePipeline}, {"tv", "tv_user", tvPipeline}} {
		if mediaType != "all" && mediaType != query.mediaType && !(mediaType == "anime" && query.mediaType == "tv") {
			continue
		}
		cursor, err := r.db.Collection(query.collection).Aggregate(ctx, query.pipeline)
		if err != nil {
			return nil, fmt.Errorf("stats: aggregate person titles: %w", err)
		}
		var titles []PersonTitle
		if err := cursor.All(ctx, &titles); err != nil {
			cursor.Close(ctx)
			return nil, fmt.Errorf("stats: decode person titles: %w", err)
		}
		cursor.Close(ctx)
		results = append(results, titles...)
	}
	return results, nil
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
			"release_date":      "$movie.release_date",
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

func (r *mongoRepository) FindWatchedTV(ctx context.Context, userID string, from, to *time.Time, isAnime *bool) ([]tvWatchRecord, error) {
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
			"is_anime":           "$tv.is_anime",
			"first_air_date":     "$tv.first_air_date",
			"episodes_in_period": "$episodesInPeriod",
		}},
	}
	if isAnime != nil {
		pipeline = append(pipeline[:6], append(bson.A{bson.M{"$match": bson.M{"tv.is_anime": *isAnime}}}, pipeline[6:]...)...)
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
