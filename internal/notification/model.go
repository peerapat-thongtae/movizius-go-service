package notification

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NotificationDevice represents a registered FCM device token
// in the notification_devices collection.
// One user may have many devices; fcm_token is unique per device.
type NotificationDevice struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	UserID    string             `bson:"user_id"       json:"user_id"`
	FCMToken  string             `bson:"fcm_token"     json:"fcm_token"`
	Platform  string             `bson:"platform"      json:"platform"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"    json:"-"`
}

// RegisterDeviceRequest is the JSON body for POST /notification/devices.
type RegisterDeviceRequest struct {
	FCMToken string `json:"fcm_token"`
	Platform string `json:"platform"`
}
