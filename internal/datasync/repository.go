package datasync

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SyncRepository is the data access contract for the sync_meta collection.
type SyncRepository interface {
	CreateSyncMeta(ctx context.Context, meta SyncMeta) error
	GetSyncMeta(ctx context.Context, syncKey string) (*SyncMeta, error)
	UpdateSyncMeta(ctx context.Context, syncKey string, metaUpdate bson.M, status string, syncDate *time.Time) error
	CountMovieIDs(ctx context.Context) (int, error)
	CountTVIDs(ctx context.Context) (int, error)
	GetMovieIDsPage(ctx context.Context, offset, limit int) ([]int64, error)
	GetTVIDsPage(ctx context.Context, offset, limit int) ([]int64, error)
}

type mongoSyncRepository struct {
	db *mongo.Database
}

// NewRepository constructs a SyncRepository backed by MongoDB.
func NewRepository(db *mongo.Database) SyncRepository {
	return &mongoSyncRepository{db: db}
}

func (r *mongoSyncRepository) CreateSyncMeta(ctx context.Context, meta SyncMeta) error {
	_, err := r.db.Collection("sync_meta").InsertOne(ctx, meta)
	if err != nil {
		return fmt.Errorf("datasync: create sync_meta: %w", err)
	}
	return nil
}

func (r *mongoSyncRepository) GetSyncMeta(ctx context.Context, syncKey string) (*SyncMeta, error) {
	var meta SyncMeta
	err := r.db.Collection("sync_meta").FindOne(ctx, bson.M{"sync_key": syncKey}).Decode(&meta)
	if err != nil {
		return nil, fmt.Errorf("datasync: get sync_meta %q: %w", syncKey, err)
	}
	return &meta, nil
}

func (r *mongoSyncRepository) UpdateSyncMeta(ctx context.Context, syncKey string, metaUpdate bson.M, status string, syncDate *time.Time) error {
	fields := bson.M{
		"meta":       metaUpdate,
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	if syncDate != nil {
		fields["sync_date"] = syncDate
	}
	_, err := r.db.Collection("sync_meta").UpdateOne(
		ctx,
		bson.M{"sync_key": syncKey},
		bson.M{"$set": fields},
		options.Update().SetUpsert(false),
	)
	if err != nil {
		return fmt.Errorf("datasync: update sync_meta %q: %w", syncKey, err)
	}
	return nil
}

func (r *mongoSyncRepository) CountMovieIDs(ctx context.Context) (int, error) {
	return countDocs(ctx, r.db.Collection("movie_user"))
}

func (r *mongoSyncRepository) CountTVIDs(ctx context.Context) (int, error) {
	return countDocs(ctx, r.db.Collection("tv_user"))
}

func (r *mongoSyncRepository) GetMovieIDsPage(ctx context.Context, offset, limit int) ([]int64, error) {
	return getIDsPage(ctx, r.db.Collection("movie_user"), offset, limit)
}

func (r *mongoSyncRepository) GetTVIDsPage(ctx context.Context, offset, limit int) ([]int64, error) {
	return getIDsPage(ctx, r.db.Collection("tv_user"), offset, limit)
}

func countDocs(ctx context.Context, coll *mongo.Collection) (int, error) {
	n, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("datasync: count %s: %w", coll.Name(), err)
	}
	return int(n), nil
}

func getIDsPage(ctx context.Context, coll *mongo.Collection, offset, limit int) ([]int64, error) {
	opts := options.Find().
		SetSkip(int64(offset)).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"_id": 0, "id": 1})

	cursor, err := coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("datasync: page %s: %w", coll.Name(), err)
	}
	defer cursor.Close(ctx)

	var docs []struct {
		ID int64 `bson:"id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("datasync: decode %s page: %w", coll.Name(), err)
	}

	ids := make([]int64, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids, nil
}
