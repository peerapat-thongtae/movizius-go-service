package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "user"

// UserRepository is the data access contract for the user collection.
type UserRepository interface {
	FindByAuth0ID(ctx context.Context, auth0ID string) (*User, error)
	UpsertNewFromAuth0(ctx context.Context, identity Identity, email string, profile Profile) error
	TouchLastLogin(ctx context.Context, auth0ID string) error
	RefreshProfile(ctx context.Context, auth0ID string, email string, profile Profile) error
}

type mongoUserRepository struct {
	db *mongo.Database
}

// NewRepository constructs a UserRepository backed by MongoDB and ensures
// the unique index on identities.auth0Id exists.
func NewRepository(db *mongo.Database) UserRepository {
	r := &mongoUserRepository{db: db}
	r.ensureIndexes()
	return r
}

func (r *mongoUserRepository) ensureIndexes() {
	coll := r.db.Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	model := mongo.IndexModel{
		Keys:    bson.D{{Key: "identities.auth0Id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("identities_auth0Id_unique"),
	}
	_, _ = coll.Indexes().CreateOne(ctx, model)
}

// FindByAuth0ID returns the user linked to the given Auth0 identity, or
// (nil, nil) if no such user exists.
func (r *mongoUserRepository) FindByAuth0ID(ctx context.Context, auth0ID string) (*User, error) {
	var u User
	err := r.db.Collection(collectionName).FindOne(ctx, bson.M{"identities.auth0Id": auth0ID}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by auth0 id %q: %w", auth0ID, err)
	}
	return &u, nil
}

// UpsertNewFromAuth0 creates a new user record on first login. If a record
// already exists for this Auth0 identity, it is left untouched.
func (r *mongoUserRepository) UpsertNewFromAuth0(ctx context.Context, identity Identity, email string, profile Profile) error {
	coll := r.db.Collection(collectionName)
	now := time.Now().UTC()

	filter := bson.M{"identities.auth0Id": identity.Auth0ID}
	update := bson.M{
		"$setOnInsert": bson.M{
			"identities":            []Identity{identity},
			"email":                 email,
			"profile":               profile,
			"preferences":           bson.M{},
			"recommendationProfile": bson.M{},
			"status":                "active",
			"createdAt":             now,
			"updatedAt":             now,
			"lastLoginAt":           now,
		},
	}

	_, err := coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("user: upsert new from auth0 %q: %w", identity.Auth0ID, err)
	}
	return nil
}

// TouchLastLogin refreshes lastLoginAt/updatedAt for an already-known user,
// without calling the Auth0 Management API.
func (r *mongoUserRepository) TouchLastLogin(ctx context.Context, auth0ID string) error {
	coll := r.db.Collection(collectionName)
	now := time.Now().UTC()

	filter := bson.M{"identities.auth0Id": auth0ID}
	update := bson.M{"$set": bson.M{"lastLoginAt": now, "updatedAt": now}}

	_, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("user: touch last login %q: %w", auth0ID, err)
	}
	return nil
}

// RefreshProfile force-updates the email and profile fields from Auth0.
func (r *mongoUserRepository) RefreshProfile(ctx context.Context, auth0ID string, email string, profile Profile) error {
	coll := r.db.Collection(collectionName)
	now := time.Now().UTC()

	filter := bson.M{"identities.auth0Id": auth0ID}
	update := bson.M{"$set": bson.M{"email": email, "profile": profile, "updatedAt": now}}

	_, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("user: refresh profile %q: %w", auth0ID, err)
	}
	return nil
}
