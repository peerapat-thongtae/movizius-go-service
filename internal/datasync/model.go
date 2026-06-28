package datasync

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	SourceUserTracked  = "user_tracked"
	SourceTMDBTrending = "tmdb_trending"

	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusNotDue     = "not_due"

	FrequencyDaily   = "daily"
	FrequencyWeekly  = "weekly"
	FrequencyMonthly = "monthly"
)

// SyncMeta persists the state of a chunked sync job in the sync_meta collection.
// Source-specific progress is stored in Meta so new sync sources need no schema changes.
type SyncMeta struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	SyncKey   string             `bson:"sync_key"`
	Source    string             `bson:"source"`              // "user_tracked" | "tmdb_trending"
	MediaType string             `bson:"media_type"`          // "movie" | "tv"
	Status    string             `bson:"status"`              // "in_progress" | "completed"
	SyncDate  *time.Time         `bson:"sync_date,omitempty"` // set when a cycle completes
	Meta      bson.M             `bson:"meta"`                // source-specific payload
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

// SyncResult is the API response shape for sync endpoints.
type SyncResult struct {
	SyncKey    string     `json:"sync_key"`
	Source     string     `json:"source"`
	Frequency  string     `json:"frequency"`
	Total      int        `json:"total"`
	Processed  int        `json:"processed"`
	Remaining  int        `json:"remaining"`
	Status     string     `json:"status"`
	NextSyncAt *time.Time `json:"next_sync_at,omitempty"`
}
