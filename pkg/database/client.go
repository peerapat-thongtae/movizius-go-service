// Package database provides the MongoDB client and connection helpers.
package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Connect dials MongoDB Atlas using the given URI and pings to confirm connectivity.
// The caller is responsible for calling client.Disconnect when done.
func Connect(ctx context.Context, uri string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return client, nil
}

// DB returns the named database from a connected client.
func DB(client *mongo.Client, name string) *mongo.Database {
	return client.Database(name)
}
