// Package firebase initializes the Firebase Admin SDK from a base64-encoded
// service account JSON stored in an environment variable.
package firebase

import (
	"context"
	"encoding/base64"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

// New decodes a base64-encoded service account JSON and returns an initialized
// Firebase App. The App is safe for concurrent use and should be created once
// at startup.
func New(serviceAccountBase64 string) (*firebase.App, error) {
	decoded, err := base64.StdEncoding.DecodeString(serviceAccountBase64)
	if err != nil {
		return nil, fmt.Errorf("firebase: decode service account: %w", err)
	}
	app, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsJSON(decoded))
	if err != nil {
		return nil, fmt.Errorf("firebase: initialize app: %w", err)
	}
	return app, nil
}
