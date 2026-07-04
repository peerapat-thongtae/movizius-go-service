// Command sync-popularity reconciles a local collection against the TMDB daily
// ID export: it refreshes popularity on titles that still exist, prunes titles
// TMDB has removed (cascading to the *_user collection), and inserts newly
// popular titles that are not yet in the catalog (fetching detail and applying
// the same acceptability filter as the regular sync). Intended to run once a day
// from a GitHub Action.
//
// Usage: sync-popularity <movie|tv>
package main

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/joho/godotenv"

	"github.com/peera/movizius-go-service/internal/movie"
	"github.com/peera/movizius-go-service/internal/tv"
	"github.com/peera/movizius-go-service/pkg/database"
	"github.com/peera/movizius-go-service/pkg/logger"
	"github.com/peera/movizius-go-service/pkg/tmdb"
)

const (
	// maxDeleteRatio aborts the delete phase if more than 10% of scanned docs
	// would be removed — a backstop against a partial or corrupt export download.
	maxDeleteRatio = 0.10

	// minInsertPopularity gates which export titles are considered for insertion:
	// only those the export marks strictly more popular than this.
	minInsertPopularity = 50.0

	// maxInsertsPerRun caps how many new titles a single run inserts (highest
	// popularity first). The backlog drains over subsequent daily runs.
	maxInsertsPerRun = 200

	// insertChunkSize bounds how many TMDB detail fetches run concurrently, since
	// the sync service fans out one goroutine per id in the slice it is given.
	insertChunkSize = 25
)

// syncer is satisfied by both *movie.MovieSyncService and *tv.TVSyncService: it
// fetches TMDB detail for each id, applies the acceptability filter, and upserts
// (inserting brand-new ids).
type syncer interface {
	Sync(ctx context.Context, ids []int64) error
}

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
	tmdbToken := os.Getenv("TMDB_API_READ_ACCESS_TOKEN")
	if tmdbToken == "" {
		log.Error("TMDB_API_READ_ACCESS_TOKEN is required")
		os.Exit(1)
	}

	// The run (download + full-collection scan + detail fetches) can take minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	client, err := database.Connect(ctx, uri)
	if err != nil {
		log.Error("mongodb connection failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := database.DB(client, "moviedb")

	tmdbClient := tmdb.New(tmdbToken)
	now := time.Now().UTC()

	// Download the export for the selected media type.
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

	// Reconcile (update + delete) and build a sync service for the insert phase,
	// both reusing the same repo instance.
	var (
		scanned, updated, deleted int64
		skippedDelete             bool
		existing                  map[int64]struct{}
		svc                       syncer
		reconcileErr              error
	)
	switch mediaType {
	case "movie":
		repo := movie.NewRepository(db)
		r, err := repo.ReconcilePopularity(ctx, export, maxDeleteRatio)
		scanned, updated, deleted, skippedDelete, existing, reconcileErr = r.Scanned, r.Updated, r.Deleted, r.SkippedDelete, r.ExistingIDs, err
		svc = movie.NewSyncService(repo, tmdbClient)
	case "tv":
		repo := tv.NewRepository(db)
		r, err := repo.ReconcilePopularity(ctx, export, maxDeleteRatio)
		scanned, updated, deleted, skippedDelete, existing, reconcileErr = r.Scanned, r.Updated, r.Deleted, r.SkippedDelete, r.ExistingIDs, err
		svc = tv.NewSyncService(repo, tmdbClient)
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

	// Insert phase: fetch detail + acceptability-filter the top new popular titles.
	candidates := topNewCandidates(export, existing, minInsertPopularity, maxInsertsPerRun)
	log.Info("insert candidates selected", "media_type", mediaType, "candidates", len(candidates))

	for start := 0; start < len(candidates); start += insertChunkSize {
		end := start + insertChunkSize
		if end > len(candidates) {
			end = len(candidates)
		}
		if err := svc.Sync(ctx, candidates[start:end]); err != nil {
			log.Error("insert chunk failed", "media_type", mediaType, "error", err)
			os.Exit(1)
		}
	}
	log.Info("insert complete", "media_type", mediaType, "candidates", len(candidates))
}

// topNewCandidates returns up to limit export ids whose popularity is strictly
// greater than minPop and which are not already present in existing, sorted by
// popularity descending (id ascending as a tie-break for determinism).
func topNewCandidates(export map[int64]float64, existing map[int64]struct{}, minPop float64, limit int) []int64 {
	type cand struct {
		id  int64
		pop float64
	}
	var cands []cand
	for id, pop := range export {
		if pop <= minPop {
			continue
		}
		if _, ok := existing[id]; ok {
			continue
		}
		cands = append(cands, cand{id: id, pop: pop})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].pop != cands[j].pop {
			return cands[i].pop > cands[j].pop
		}
		return cands[i].id < cands[j].id
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}
	ids := make([]int64, len(cands))
	for i, c := range cands {
		ids[i] = c.id
	}
	return ids
}
