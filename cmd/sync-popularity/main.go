// Command sync-popularity reconciles the local movie collection against the TMDB
// daily movie-ID export: it refreshes popularity on movies that still exist and
// prunes movies TMDB has removed (cascading to movie_user). It performs no
// inserts. Intended to run once a day from a GitHub Action.
package main

import (
	"context"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/pkg/database"
	"github.com/peera/movizius-go-service/pkg/logger"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// maxDeleteRatio aborts the delete phase if more than 10% of scanned movies
// would be removed — a backstop against a partial or corrupt export download.
const maxDeleteRatio = 0.10

func main() {
	// Load .env if present — no-op when env vars are already set (e.g. CI).
	_ = godotenv.Load()

	log := logger.New()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Error("MONGO_URI is required")
		os.Exit(1)
	}

	// The whole run (download + full-collection scan) can take a few minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	client, err := database.Connect(ctx, uri)
	if err != nil {
		log.Error("mongodb connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := database.DB(client, "moviedb")

	log.Info("downloading TMDB movie-id export")
	export, err := tmdb.FetchMovieIDPopularity(ctx, time.Now().UTC())
	if err != nil {
		log.Error("failed to fetch export", "error", err)
		os.Exit(1)
	}
	log.Info("export downloaded", "rows", len(export))

	repo := movie.NewRepository(db)
	res, err := repo.ReconcilePopularity(ctx, export, maxDeleteRatio)
	if err != nil {
		log.Error("reconcile failed", "error", err)
		os.Exit(1)
	}

	log.Info("reconcile complete",
		"export_rows", len(export),
		"scanned", res.Scanned,
		"updated", res.Updated,
		"deleted", res.Deleted,
		"skipped_delete", res.SkippedDelete,
	)
}
