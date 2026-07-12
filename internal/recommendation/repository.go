package recommendation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// userCollection is the Mongo collection recommendation profiles are stored
// on (internal/user.User.RecommendationProfile). Referenced by raw name
// rather than importing internal/user, since internal/user imports this
// package's Profile type — importing user here would create a cycle.
const userCollection = "user"

// Repository is the data access contract for persisted recommendation profiles.
type Repository interface {
	// GetProfile returns the profile for a user, or a zero-value Profile if
	// the user has none yet (including users whose recommendationProfile is
	// still the legacy empty placeholder).
	GetProfile(ctx context.Context, userID string) (Profile, error)
	// ApplyProfileUpdate incrementally applies bucket deltas from a single
	// watch-state-change event to a user's profile, recomputing scores for
	// just the touched entities. firstTouch must be true only the first time
	// a given title ever contributes (controls count increments); watchedID,
	// if non-nil, is added to the media type's watchedIds exclusion list.
	ApplyProfileUpdate(ctx context.Context, userID string, mediaType MediaType, deltas []BucketDelta, firstTouch bool, watchedID *int64) error
	// ReplaceProfile overwrites a user's entire recommendationProfile (used by full recompute).
	ReplaceProfile(ctx context.Context, userID string, p Profile) error
}

type mongoRepository struct {
	db *mongo.Database
}

// NewRepository constructs a Repository backed by MongoDB.
func NewRepository(db *mongo.Database) Repository {
	return &mongoRepository{db: db}
}

func entityKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (r *mongoRepository) GetProfile(ctx context.Context, userID string) (Profile, error) {
	var doc struct {
		RecommendationProfile Profile `bson:"recommendationProfile"`
	}
	err := r.db.Collection(userCollection).FindOne(ctx, bson.M{"id": userID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Profile{}, nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("recommendation: get profile %q: %w", userID, err)
	}
	return doc.RecommendationProfile, nil
}

func (r *mongoRepository) ApplyProfileUpdate(ctx context.Context, userID string, mediaType MediaType, deltas []BucketDelta, firstTouch bool, watchedID *int64) error {
	if len(deltas) == 0 && watchedID == nil {
		return nil
	}
	coll := r.db.Collection(userCollection)
	filter := bson.M{"id": userID}
	now := time.Now().UTC()

	if len(deltas) > 0 {
		inc := bson.M{}
		for _, d := range deltas {
			base := fmt.Sprintf("recommendationProfile.%s.%s.%s", mediaType, d.Bucket, entityKey(d.EntityID))
			inc[base+".rawSum"] = d.RawSumDelta
			if firstTouch {
				inc[base+".count"] = 1
			}
		}
		if firstTouch {
			inc["recommendationProfile.meta.sourceEventCount"] = 1
		}
		update := bson.M{
			"$inc": inc,
			"$set": bson.M{
				"recommendationProfile.version":             ScoringVersion,
				"recommendationProfile.meta.scoringVersion": ScoringVersion,
				"recommendationProfile.updatedAt":           now,
			},
		}
		if _, err := coll.UpdateOne(ctx, filter, update); err != nil {
			return fmt.Errorf("recommendation: apply profile deltas %q: %w", userID, err)
		}

		// Re-read the touched entities to recompute their scores from the
		// post-increment rawSum/count, then $set the scores in a second
		// round trip (score isn't itself incrementable).
		profile, err := r.GetProfile(ctx, userID)
		if err != nil {
			return err
		}
		mediaProfile := profile.Movie
		if mediaType == MediaTypeTV {
			mediaProfile = profile.TV
		}
		scoreSet := bson.M{}
		seen := map[string]bool{}
		for _, d := range deltas {
			key := d.Bucket + "." + entityKey(d.EntityID)
			if seen[key] {
				continue
			}
			seen[key] = true
			bucket := bucketByName(mediaProfile, d.Bucket)
			entry := bucket[entityKey(d.EntityID)]
			scorePath := fmt.Sprintf("recommendationProfile.%s.%s.%s.score", mediaType, d.Bucket, entityKey(d.EntityID))
			scoreSet[scorePath] = Score(entry.RawSum, entry.Count)
		}
		if len(scoreSet) > 0 {
			if _, err := coll.UpdateOne(ctx, filter, bson.M{"$set": scoreSet}); err != nil {
				return fmt.Errorf("recommendation: set recomputed scores %q: %w", userID, err)
			}
		}
	}

	if watchedID != nil {
		watchedPath := fmt.Sprintf("recommendationProfile.%s.watchedIds", mediaType)
		update := bson.M{
			"$addToSet": bson.M{watchedPath: *watchedID},
			"$set":      bson.M{"recommendationProfile.updatedAt": now},
		}
		if _, err := coll.UpdateOne(ctx, filter, update); err != nil {
			return fmt.Errorf("recommendation: add watched id %q: %w", userID, err)
		}
	}

	return nil
}

// bucketByName returns the Bucket for the given bucket-name constant.
func bucketByName(mp MediaProfile, name string) Bucket {
	switch name {
	case BucketGenres:
		return mp.Genres
	case BucketKeywords:
		return mp.Keywords
	case BucketActors:
		return mp.Actors
	case BucketDirectors:
		return mp.Directors
	case BucketCreators:
		return mp.Creators
	case BucketCollections:
		return mp.Collections
	case BucketProductionCompanies:
		return mp.ProductionCompanies
	default:
		return nil
	}
}

func (r *mongoRepository) ReplaceProfile(ctx context.Context, userID string, p Profile) error {
	filter := bson.M{"id": userID}
	update := bson.M{"$set": bson.M{"recommendationProfile": p}}
	if _, err := r.db.Collection(userCollection).UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("recommendation: replace profile %q: %w", userID, err)
	}
	return nil
}
