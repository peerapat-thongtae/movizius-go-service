// Package middleware provides HTTP middleware for the API.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/peera/movizius-go-service/internal/shared/response"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const userIDKey contextKey = "auth0_user_id"

// NewJWKS fetches and caches the Auth0 JWKS from {issuerURL}.well-known/jwks.json.
// The returned *keyfunc.JWKS is safe for concurrent use and should be created once at startup.
func NewJWKS(issuerURL string) (keyfunc.Keyfunc, error) {
	jwksURL := strings.TrimRight(issuerURL, "/") + "/.well-known/jwks.json"
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", jwksURL, err)
	}
	return k, nil
}

// RequireAuth returns middleware that validates Auth0 Bearer JWTs.
// On success it stores the Auth0 sub claim in the request context.
// On failure it writes a 401 response and stops the chain.
func RequireAuth(kf keyfunc.Keyfunc, issuer, audience string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractAndValidate(r, kf, issuer, audience)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			sub, err := token.Claims.GetSubject()
			if err != nil || sub == "" {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the Auth0 user ID (sub claim) stored by RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok && v != ""
}

func extractAndValidate(r *http.Request, kf keyfunc.Keyfunc, issuer, audience string) (*jwt.Token, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, fmt.Errorf("missing bearer token")
	}
	raw := strings.TrimPrefix(header, "Bearer ")

	token, err := jwt.Parse(raw, kf.Keyfunc,
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token not valid")
	}
	return token, nil
}
