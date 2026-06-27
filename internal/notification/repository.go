package notification

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "notification_devices"

// NotificationRepository is the data access contract for the notification_devices collection.
type NotificationRepository interface {
	UpsertDevice(ctx context.Context, device NotificationDevice) error
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
