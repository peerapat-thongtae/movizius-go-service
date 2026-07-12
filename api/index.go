package handler

import (
	"context"
	"log"
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/router"
	pkgauth0 "github.com/peera/movizius-go-service/pkg/auth0"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/config"
	"github.com/peera/movizius-go-service/pkg/database"
	pkgfirebase "github.com/peera/movizius-go-service/pkg/firebase"
	"github.com/peera/movizius-go-service/pkg/logger"
	"github.com/peera/movizius-go-service/pkg/tmdb"
	"github.com/peera/movizius-go-service/pkg/tvmaze"
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

	auth0Client, err := pkgauth0.New(context.Background(), cfg.Auth0Domain, cfg.Auth0ClientID, cfg.Auth0ClientSecret)
	if err != nil {
		log.Fatalf("auth0: %v", err)
	}

	app = router.New(router.Deps{
		DB:             database.DB(client, "moviedb"),
		Cache:          cache.NewUpstash(cfg.UpstashURL, cfg.UpstashToken),
		JWKS:           jwks,
		Auth0IssuerURL: cfg.Auth0IssuerURL,
		Auth0Audience:  cfg.Auth0Audience,
		Auth0:          auth0Client,
		Firebase:       firebaseApp,
		TMDB:           tmdb.New(cfg.TMDBAccessToken),
		TVMaze:         tvmaze.New(""),
		Logger:         logger.New(),
		Development:    cfg.IsDevelopment(),
	})
}

// Handler is the Vercel Go Serverless Function entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	app.ServeHTTP(w, r)
}
