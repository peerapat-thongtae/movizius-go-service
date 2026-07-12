package user

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Identity links a local user record to an Auth0 identity (one per connection/provider).
type Identity struct {
	Provider string `bson:"provider" json:"provider"`
	Auth0ID  string `bson:"auth0Id"  json:"auth0Id"`
}

// Profile holds display information synced from Auth0.
type Profile struct {
	Name   string `bson:"name"   json:"name"`
	Avatar string `bson:"avatar" json:"avatar"`
}

// User represents a document in the "user" collection.
type User struct {
	ID                    primitive.ObjectID `bson:"_id,omitempty"          json:"id"`
	Identities            []Identity         `bson:"identities"             json:"identities"`
	Email                 string             `bson:"email"                  json:"email"`
	Profile               Profile            `bson:"profile"                json:"profile"`
	Preferences           bson.M             `bson:"preferences"            json:"preferences"`
	RecommendationProfile bson.M             `bson:"recommendationProfile"  json:"recommendationProfile"`
	Status                string             `bson:"status"                 json:"status"`
	CreatedAt             time.Time          `bson:"createdAt"              json:"createdAt"`
	UpdatedAt             time.Time          `bson:"updatedAt"              json:"updatedAt"`
	LastLoginAt           time.Time          `bson:"lastLoginAt"            json:"lastLoginAt"`
}
