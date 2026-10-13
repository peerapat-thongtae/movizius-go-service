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
