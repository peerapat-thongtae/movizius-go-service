// Package router builds the application's HTTP router and wires feature routes.
package router

import (
	"net/http"

	"github.com/MicahParks/keyfunc/v3"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/peera/movizius-go-service/internal/health"
	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/internal/tv"
	"github.com/peera/movizius-go-service/pkg/cache"
)

// Deps holds the shared infrastructure dependencies injected into feature handlers.
type Deps struct {
	DB             *mongo.Database
	Cache          cache.Cache
	JWKS           keyfunc.Keyfunc
	Auth0IssuerURL string
	Auth0Audience  string
}

// New constructs the application handler with all feature routes registered under /api.
// Go's ServeMux automatically redirects /api → /api/ so both forms work.
func New(deps Deps) http.Handler {
	mux := http.NewServeMux()
	auth := middleware.RequireAuth(deps.JWKS, deps.Auth0IssuerURL, deps.Auth0Audience)

	// Root hello-world.
	mux.HandleFunc("GET /{$}", root)

	// Public routes (no auth).
	health.NewHandler(health.NewService()).RegisterRoutes(mux)

	// Protected routes — each feature applies auth to its own handlers.
	movie.NewHandler(movie.NewService(movie.NewRepository(deps.DB))).RegisterRoutes(mux, auth)
	tv.NewHandler(tv.NewService(tv.NewRepository(deps.DB))).RegisterRoutes(mux, auth)

	// Mount the inner mux under /api/. StripPrefix removes /api before the inner
	// mux sees the path, so features register routes without the base prefix.
	outer := http.NewServeMux()
	outer.Handle("/api/", http.StripPrefix("/api", mux))
	return outer
}

func root(w http.ResponseWriter, r *http.Request) {
	response.Success(w, http.StatusOK, map[string]string{"message": "hello world"})
}
