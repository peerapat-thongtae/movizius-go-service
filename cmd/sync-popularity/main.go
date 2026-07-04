// Command sync-popularity reconciles a local collection against the TMDB daily
// ID export: it refreshes popularity on titles that still exist and prunes
// titles TMDB has removed (cascading to the *_user collection). It performs no
// inserts. Intended to run once a day from a GitHub Action.
//
// Usage: sync-popularity <movie|tv>
package main

import (
	"context"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/tv"
	"github.com/peera/movizius-go-service/pkg/database"
	"github.com/peera/movizius-go-service/pkg/logger"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

// maxDeleteRatio aborts the delete phase if more than 10% of scanned docs would
// be removed — a backstop against a partial or corrupt export download.
const maxDeleteRatio = 0.10

func main() {
	// Load .env if present — no-op when env vars are already set (e.g. CI).
	_ = godotenv.Load()

	log := logger.New()

	if len(os.Args) < 2 {
		log.Error("media type argument is required", "usage", "sync-popularity <movie|tv>")
		os.Exit(1)
	}
	mediaType := os.Args[1]
	if mediaType != "movie" && mediaType != "tv" {
		log.Error("invalid media type", "got", mediaType, "want", "movie or tv")
		os.Exit(1)
	}

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Error("MONGO_URI is required")
		os.Exit(1)
	}

	// The run (download + full-collection scan) can take several minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	client, err := database.Connect(ctx, uri)
	if err != nil {
		log.Error("mongodb connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := database.DB(client, "moviedb")

	now := time.Now().UTC()

	// fetch downloads the export; reconcile applies it. Both are selected by media
	// type so the movie and tv jobs share one binary but run independently.
	var (
		export   map[int64]float64
		fetchErr error
	)
	switch mediaType {
	case "movie":
		export, fetchErr = tmdb.FetchMovieIDPopularity(ctx, now)
	case "tv":
		export, fetchErr = tmdb.FetchTVIDPopularity(ctx, now)
	}
	if fetchErr != nil {
		log.Error("failed to fetch export", "media_type", mediaType, "error", fetchErr)
		os.Exit(1)
	}
	log.Info("export downloaded", "media_type", mediaType, "rows", len(export))

	var (
		scanned, updated, deleted int64
		skippedDelete             bool
		reconcileErr              error
	)
	switch mediaType {
	case "movie":
		res, err := movie.NewRepository(db).ReconcilePopularity(ctx, export, maxDeleteRatio)
		scanned, updated, deleted, skippedDelete, reconcileErr = res.Scanned, res.Updated, res.Deleted, res.SkippedDelete, err
	case "tv":
		res, err := tv.NewRepository(db).ReconcilePopularity(ctx, export, maxDeleteRatio)
		scanned, updated, deleted, skippedDelete, reconcileErr = res.Scanned, res.Updated, res.Deleted, res.SkippedDelete, err
	}
	if reconcileErr != nil {
		log.Error("reconcile failed", "media_type", mediaType, "error", reconcileErr)
		os.Exit(1)
	}

	log.Info("reconcile complete",
		"media_type", mediaType,
		"export_rows", len(export),
		"scanned", scanned,
		"updated", updated,
		"deleted", deleted,
		"skipped_delete", skippedDelete,
	)
}
