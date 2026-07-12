// Package router builds the application's HTTP router and wires feature routes.
package router

import (
	"log/slog"
	"net/http"

	firebase "firebase.google.com/go/v4"
	"github.com/MicahParks/keyfunc/v3"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.mongodb.org/mongo-driver/mongo"

	_ "github.com/peera/movizius-go-service/docs"
	"github.com/peera/movizius-go-service/internal/datasync"
	"github.com/peera/movizius-go-service/internal/health"
	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/notification"
	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
	"github.com/peera/movizius-go-service/internal/tv"
	"github.com/peera/movizius-go-service/internal/user"
	"github.com/peera/movizius-go-service/pkg/auth0"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/tmdb"
	"github.com/peera/movizius-go-service/pkg/tvmaze"
)

// Deps holds the shared infrastructure dependencies injected into feature handlers.
type Deps struct {
	DB             *mongo.Database
	Cache          cache.Cache
	JWKS           keyfunc.Keyfunc
	Auth0IssuerURL string
	Auth0Audience  string
	Auth0          *auth0.Client
	Firebase       *firebase.App
	TMDB           *tmdb.Client
	TVMaze         *tvmaze.Client
	Logger         *slog.Logger
	Development    bool
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
	// Swagger UI is only served in development.
	if deps.Development {
		// The outer mux strips /api from r.URL.Path but not r.RequestURI.
		// httpSwagger uses r.RequestURI to detect its prefix, then strips it from r.URL.Path —
		// if they diverge the asset paths don't match and return 404.
		// Cloning the request with RequestURI = r.URL.RequestURI() re-aligns them.
		swaggerUI := httpSwagger.Handler(httpSwagger.URL("/api/swagger/doc.json"))
		mux.Handle("/swagger/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r2 := r.Clone(r.Context())
			r2.RequestURI = r.URL.RequestURI()
			swaggerUI(w, r2)
		}))
	}

	// Protected routes — each feature applies auth to its own handlers.
	movieRepo := movie.NewRepository(deps.DB)
	tvRepo := tv.NewRepository(deps.DB)

	// authSynced wraps auth with a lazy user sync: every authenticated request
	// upserts/refreshes the caller's user record from Auth0.
	userService := user.NewService(user.NewRepository(deps.DB), deps.Auth0, deps.Logger)
	authSynced := func(next http.Handler) http.Handler {
		return auth(user.SyncMiddleware(userService, deps.Logger)(next))
	}

	user.NewHandler(userService, deps.Logger).RegisterRoutes(mux, authSynced)
	movie.NewHandler(movie.NewService(movieRepo, deps.TMDB, deps.Cache), deps.Logger).RegisterRoutes(mux, authSynced)
	tv.NewHandler(tv.NewService(tvRepo, deps.TMDB, deps.Cache), deps.Logger).RegisterRoutes(mux, authSynced)
	notification.NewHandler(notification.NewService(notification.NewRepository(deps.DB), deps.Firebase), deps.Logger).RegisterRoutes(mux, authSynced)

	datasync.NewHandler(datasync.NewService(
		datasync.NewRepository(deps.DB),
		movie.NewSyncService(movieRepo, deps.TMDB),
		tv.NewSyncService(tvRepo, deps.TMDB),
		deps.TMDB,
		deps.TVMaze,
	), deps.Logger).RegisterRoutes(mux, authSynced)

	// Mount the inner mux under /api/. StripPrefix removes /api before the inner
	// mux sees the path, so features register routes without the base prefix.
	outer := http.NewServeMux()
	outer.Handle("/api/", http.StripPrefix("/api", mux))

	// Recover wraps every route: panic safety net + 5xx access logging.
	return middleware.Recover(deps.Logger)(outer)
}

func root(w http.ResponseWriter, r *http.Request) {
	response.Success(w, http.StatusOK, map[string]string{"message": "hello world"})
}
