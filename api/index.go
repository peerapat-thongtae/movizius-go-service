package handler

import (
	"context"
	"log"
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/router"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/config"
	"github.com/peera/movizius-go-service/pkg/database"
	pkgfirebase "github.com/peera/movizius-go-service/pkg/firebase"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// app is built once per cold start and reused across warm invocations.
var app http.Handler

func init() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	client, err := database.Connect(context.Background(), cfg.MongoURI)
	if err != nil {
		log.Fatalf("mongodb: %v", err)
	}

	jwks, err := middleware.NewJWKS(cfg.Auth0IssuerURL)
	if err != nil {
		log.Fatalf("jwks: %v", err)
	}

	firebaseApp, err := pkgfirebase.New(cfg.FirebaseServiceAccountBase64)
	if err != nil {
		log.Fatalf("firebase: %v", err)
	}

	app = router.New(router.Deps{
		DB:             database.DB(client, "moviedb"),
		Cache:          cache.NewUpstash(cfg.UpstashURL, cfg.UpstashToken),
		JWKS:           jwks,
		Auth0IssuerURL: cfg.Auth0IssuerURL,
		Auth0Audience:  cfg.Auth0Audience,
		Firebase:       firebaseApp,
		TMDB:           tmdb.New(cfg.TMDBAccessToken),
	})
}

// Handler is the Vercel Go Serverless Function entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	app.ServeHTTP(w, r)
}
