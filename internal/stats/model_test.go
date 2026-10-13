package stats

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestPersonTitleDecodesMediaTypeFromMongo(t *testing.T) {
	watchedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	raw, err := bson.Marshal(bson.M{
		"id":             int64(263089),
		"media_type":     "tv",
		"title":          "A series",
		"poster_path":    "/poster.jpg",
		"first_air_date": "2025-01-01",
		"watched_at":     watchedAt,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	var title PersonTitle
	if err := bson.Unmarshal(raw, &title); err != nil {
		t.Fatalf("unmarshal title: %v", err)
	}
	if title.MediaType != "tv" {
		t.Fatalf("media type = %q, want tv", title.MediaType)
	}
	if title.ID != 263089 || !title.WatchedAt.Equal(watchedAt) {
		t.Fatalf("decoded unexpected title: %#v", title)
	}
}

func TestWatchedAtMatchUsesSummaryYearBounds(t *testing.T) {
	from, to := (SummaryQuery{Period: "year", Year: 2026}).dateRange()
	match := watchedAtMatch(from, to)

	wantFrom := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if got := match["$gte"]; got != wantFrom {
		t.Fatalf("start = %v, want %v", got, wantFrom)
	}
	if got := match["$lt"]; got != wantTo {
		t.Fatalf("exclusive end = %v, want %v", got, wantTo)
	}
	if allTime := watchedAtMatch(nil, nil); len(allTime) != 2 {
		t.Fatalf("all-time predicate unexpectedly has date bounds: %v", allTime)
	}
}
