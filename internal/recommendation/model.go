// Package recommendation computes and maintains a per-user recommendation
// profile from watch history (movies & TV), based on TMDB metadata. The
// profile is a set of weighted scores per entity type (genre, keyword,
// actor, director/creator, collection, production company), used to rank
// candidate titles for recommendations.
package recommendation

import "time"

// ScoringVersion must bump whenever the weighting formula or its constants
// change, so stale profiles can be identified and reconciled via a full
// recompute.
const ScoringVersion = 1

// BucketEntry is one entity's accumulated score within a bucket.
type BucketEntry struct {
	Score  int     `bson:"score"  json:"score"`
	Count  int     `bson:"count"  json:"count"`
	RawSum float64 `bson:"rawSum" json:"-"` // internal only; stripped before API response
}

// Bucket maps an entity id (as a string, since TMDB ids are used as literal
// map/object keys) to its accumulated entry.
type Bucket map[string]BucketEntry

// MediaProfile holds all buckets for one media type (movie or tv).
type MediaProfile struct {
	Genres              Bucket  `bson:"genres"                json:"genres"`
	Keywords            Bucket  `bson:"keywords"              json:"keywords"`
	Actors              Bucket  `bson:"actors"                json:"actors"`
	Directors           Bucket  `bson:"directors,omitempty"   json:"directors,omitempty"`   // movie only
	Creators            Bucket  `bson:"creators,omitempty"    json:"creators,omitempty"`    // tv only
	Collections         Bucket  `bson:"collections,omitempty" json:"collections,omitempty"` // movie only
	ProductionCompanies Bucket  `bson:"productionCompanies"   json:"productionCompanies"`
	WatchedIDs          []int64 `bson:"watchedIds"            json:"watchedIds"`
}

// Meta holds profile-level bookkeeping, not per-entity.
type Meta struct {
	TotalMovieWatched int     `bson:"totalMovieWatched" json:"totalMovieWatched"`
	TotalTvWatched    int     `bson:"totalTvWatched"    json:"totalTvWatched"`
	DecayHalfLifeDays float64 `bson:"decayHalfLifeDays" json:"decayHalfLifeDays"`
	SourceEventCount  int     `bson:"sourceEventCount"  json:"sourceEventCount"`
	ScoringVersion    int     `bson:"scoringVersion"    json:"scoringVersion"`
}

// Profile is the full recommendationProfile document embedded in user.User.
type Profile struct {
	Version   int          `bson:"version"   json:"version"`
	Movie     MediaProfile `bson:"movie"     json:"movie"`
	TV        MediaProfile `bson:"tv"        json:"tv"`
	Meta      Meta         `bson:"meta"      json:"meta"`
	UpdatedAt time.Time    `bson:"updatedAt" json:"updatedAt"`
}

// MediaType enumerates the two media types a profile tracks.
type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
)

// Bucket name constants, used as both struct-independent keys in aggregate.go
// and as the dynamic Mongo field-path segments in repository.go.
const (
	BucketGenres              = "genres"
	BucketKeywords            = "keywords"
	BucketActors              = "actors"
	BucketDirectors           = "directors"
	BucketCreators            = "creators"
	BucketCollections         = "collections"
	BucketProductionCompanies = "productionCompanies"
)
