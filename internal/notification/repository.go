package notification

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "notification_devices"

// NotificationRepository is the data access contract for the notification_devices collection.
type NotificationRepository interface {
	UpsertDevice(ctx context.Context, device NotificationDevice) error
	FindAll(ctx context.Context) ([]NotificationDevice, error)
	FindUsersWithAiringToday(ctx context.Context) ([]UserAiringShows, error)
	FindUsersWithMovieReleasingToday(ctx context.Context) ([]UserAiringShows, error)
	FindDevicesByUserIDs(ctx context.Context, userIDs []string) (map[string][]string, error)
}

type mongoNotificationRepository struct {
	db *mongo.Database
}

// NewRepository constructs a NotificationRepository backed by MongoDB and
// ensures the unique index on fcm_token exists.
func NewRepository(db *mongo.Database) NotificationRepository {
	r := &mongoNotificationRepository{db: db}
	r.ensureIndexes()
	return r
}

// ensureIndexes creates the unique index on fcm_token if it does not already exist.
func (r *mongoNotificationRepository) ensureIndexes() {
	coll := r.db.Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	model := mongo.IndexModel{
		Keys:    bson.D{{Key: "fcm_token", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("fcm_token_unique"),
	}
	_, _ = coll.Indexes().CreateOne(ctx, model)
}

// FindAll returns all registered device records.
func (r *mongoNotificationRepository) FindAll(ctx context.Context) ([]NotificationDevice, error) {
	coll := r.db.Collection(collectionName)
	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("notification: find all: %w", err)
	}
	defer cursor.Close(ctx)

	var devices []NotificationDevice
	if err := cursor.All(ctx, &devices); err != nil {
		return nil, fmt.Errorf("notification: decode devices: %w", err)
	}
	return devices, nil
}

// UpsertDevice inserts or updates the device row keyed on fcm_token.
// created_at is set only on first insert; user_id, platform, and updated_at
// are always refreshed.
func (r *mongoNotificationRepository) UpsertDevice(ctx context.Context, device NotificationDevice) error {
	coll := r.db.Collection(collectionName)
	now := time.Now().UTC()

	filter := bson.M{"fcm_token": device.FCMToken}
	update := bson.M{
		"$set": bson.M{
			"user_id":    device.UserID,
			"platform":   device.Platform,
			"updated_at": now,
		},
		"$setOnInsert": bson.M{
			"fcm_token":  device.FCMToken,
			"created_at": now,
		},
	}

	_, err := coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("notification: upsert device: %w", err)
	}
	return nil
}

// FindUsersWithAiringToday aggregates shows airing today from the tv collection,
// joins tv_user, and returns a per-user list of show names sorted by popularity desc.
func bangkokToday() string {
	loc, _ := time.LoadLocation("Asia/Bangkok")
	return time.Now().In(loc).Format("2006-01-02")
}

func (r *mongoNotificationRepository) FindUsersWithAiringToday(ctx context.Context) ([]UserAiringShows, error) {
	today := bangkokToday()
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.M{
			"next_episode_to_air.air_date": primitive.Regex{Pattern: "^" + today, Options: ""},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "popularity", Value: -1}}}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "tv_user"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "as", Value: "users"},
		}}},
		bson.D{{Key: "$unwind", Value: "$users"}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$users.user_id"},
			{Key: "shows", Value: bson.D{{Key: "$push", Value: bson.D{{Key: "name", Value: "$name"}}}}},
		}}},
	}

	cursor, err := r.db.Collection("tv").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("notification: airing today aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	var results []UserAiringShows
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("notification: decode airing today: %w", err)
	}
	return results, nil
}

// FindUsersWithMovieReleasingToday aggregates movies releasing today from the movie collection,
// joins movie_user, and returns a per-user list of movie titles sorted by popularity desc.
// release_date_th takes priority; falls back to release_date.
func (r *mongoNotificationRepository) FindUsersWithMovieReleasingToday(ctx context.Context) ([]UserAiringShows, error) {
	today := bangkokToday()
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.M{
			"$or": bson.A{
				bson.M{"release_date_th": primitive.Regex{Pattern: "^" + today, Options: ""}},
				bson.M{
					"release_date_th": bson.M{"$in": bson.A{nil, ""}},
					"release_date":    primitive.Regex{Pattern: "^" + today, Options: ""},
				},
			},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "popularity", Value: -1}}}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "movie_user"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "id"},
			{Key: "as", Value: "users"},
		}}},
		bson.D{{Key: "$unwind", Value: "$users"}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$users.user_id"},
			{Key: "shows", Value: bson.D{{Key: "$push", Value: bson.D{{Key: "name", Value: "$title"}}}}},
		}}},
	}

	cursor, err := r.db.Collection("movie").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("notification: movie releasing today aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	var results []UserAiringShows
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("notification: decode movie releasing today: %w", err)
	}
	return results, nil
}

// FindDevicesByUserIDs returns a map of user_id → FCM token list for the given user IDs.
func (r *mongoNotificationRepository) FindDevicesByUserIDs(ctx context.Context, userIDs []string) (map[string][]string, error) {
	cursor, err := r.db.Collection(collectionName).Find(ctx, bson.M{"user_id": bson.M{"$in": userIDs}})
	if err != nil {
		return nil, fmt.Errorf("notification: find devices by user ids: %w", err)
	}
	defer cursor.Close(ctx)

	var devices []NotificationDevice
	if err := cursor.All(ctx, &devices); err != nil {
		return nil, fmt.Errorf("notification: decode devices by user ids: %w", err)
	}

	result := make(map[string][]string, len(userIDs))
	for _, d := range devices {
		result[d.UserID] = append(result[d.UserID], d.FCMToken)
	}
	return result, nil
}
