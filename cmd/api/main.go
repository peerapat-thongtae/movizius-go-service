// Command api runs the Movizius HTTP server locally (and on non-Vercel hosts).
//
//	@title			Movizius API
//	@version		1.0
//	@description	Movie & TV Series tracking backend.
//
//	@contact.name	Movizius
//	@contact.email	peera.thongtae@gmail.com
//
//	@host		localhost:8080
//	@BasePath	/api
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				JWT bearer token — format: "Bearer <token>"
package main

import (
	"context"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/router"
	"github.com/peera/movizius-go-service/pkg/cache"
	"github.com/peera/movizius-go-service/pkg/config"
	"github.com/peera/movizius-go-service/pkg/database"
	pkgfirebase "github.com/peera/movizius-go-service/pkg/firebase"
	"github.com/peera/movizius-go-service/pkg/logger"
	"github.com/peera/movizius-go-service/pkg/tmdb"
	"github.com/peera/movizius-go-service/pkg/tvmaze"
)

func main() {
	// Load .env if present — no-op when env vars are already set (e.g. production).
	_ = godotenv.Load()

	log := logger.New()

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "error", err)
		os.Exit(1)
	}

	client, err := database.Connect(context.Background(), cfg.MongoURI)
	if err != nil {
		log.Error("mongodb connection failed", "error", err)
		os.Exit(1)
	}
	log.Info("mongodb connected")

	db := database.DB(client, "moviedb")

	jwks, err := middleware.NewJWKS(cfg.Auth0IssuerURL)
	if err != nil {
		log.Error("failed to fetch JWKS", "error", err)
		os.Exit(1)
	}
	log.Info("jwks loaded", "issuer", cfg.Auth0IssuerURL)

	firebaseApp, err := pkgfirebase.New(cfg.FirebaseServiceAccountBase64)
	if err != nil {
		log.Error("failed to initialize firebase", "error", err)
		os.Exit(1)
	}
	log.Info("firebase initialized")

	addr := ":" + cfg.Port
	log.Info("starting server", "addr", addr)

	if err := http.ListenAndServe(addr, router.New(router.Deps{
		DB:             db,
		Cache:          cache.NewUpstash(cfg.UpstashURL, cfg.UpstashToken),
		JWKS:           jwks,
		Auth0IssuerURL: cfg.Auth0IssuerURL,
		Auth0Audience:  cfg.Auth0Audience,
		Firebase:       firebaseApp,
		TMDB:           tmdb.New(cfg.TMDBAccessToken),
		TVMaze:         tvmaze.New(""),
		Logger:         log,
	})); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
