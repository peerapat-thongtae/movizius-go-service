package recommendation

// EntityRefs is the set of TMDB entity ids a title belongs to, extracted
// from cached Movie/TV metadata, used to fan a single event's contribution
// out to buckets. Movie-only fields (DirectorID, CollectionID) and TV-only
// fields (CreatorIDs) are left zero-valued by the caller when not applicable.
type EntityRefs struct {
	Genres              []int64
	Keywords            []int64
	CastIDs             []int64 // already capped to top billing by the movie/tv mapper
	DirectorID          *int64  // movie only
	CreatorIDs          []int64 // tv only
	CollectionID        *int64  // movie only
	ProductionCompanies []int64
}

// DefaultMultiplier applies to buckets with no dedicated coefficient
// (genres, keywords, collections, production companies).
const DefaultMultiplier = 1.0

// Multipliers holds the per-bucket weighting coefficients used by Deltas,
// sourced from Config so they're tunable via env vars without a redeploy.
type Multipliers struct {
	LeadActor float64 // cast index 0
	Actor     float64 // cast index 1-4
	Director  float64
	Creator   float64
}

// BucketDelta is one (bucket, entityID) -> rawSum delta produced by fanning
// out a single event's contribution.
type BucketDelta struct {
	Bucket      string
	EntityID    int64
	RawSumDelta float64
}

// Deltas fans a signed contribution delta out into every bucket/entity a
// title belongs to, applying per-bucket multipliers. Returns nil if
// contribution is 0 (no-op short-circuit, used by the caller for
// idempotency). Does not know about count/first-touch semantics — that
// depends on per-title history the caller (service.go) tracks separately.
func Deltas(contribution float64, refs EntityRefs, m Multipliers) []BucketDelta {
	if contribution == 0 {
		return nil
	}

	var deltas []BucketDelta

	for _, id := range refs.Genres {
		deltas = append(deltas, BucketDelta{Bucket: BucketGenres, EntityID: id, RawSumDelta: contribution * DefaultMultiplier})
	}
	for _, id := range refs.Keywords {
		deltas = append(deltas, BucketDelta{Bucket: BucketKeywords, EntityID: id, RawSumDelta: contribution * DefaultMultiplier})
	}
	for i, id := range refs.CastIDs {
		mult := m.Actor
		if i == 0 {
			mult = m.LeadActor
		}
		deltas = append(deltas, BucketDelta{Bucket: BucketActors, EntityID: id, RawSumDelta: contribution * mult})
	}
	if refs.DirectorID != nil {
		deltas = append(deltas, BucketDelta{Bucket: BucketDirectors, EntityID: *refs.DirectorID, RawSumDelta: contribution * m.Director})
	}
	for _, id := range refs.CreatorIDs {
		deltas = append(deltas, BucketDelta{Bucket: BucketCreators, EntityID: id, RawSumDelta: contribution * m.Creator})
	}
	if refs.CollectionID != nil {
		deltas = append(deltas, BucketDelta{Bucket: BucketCollections, EntityID: *refs.CollectionID, RawSumDelta: contribution * DefaultMultiplier})
	}
	for _, id := range refs.ProductionCompanies {
		deltas = append(deltas, BucketDelta{Bucket: BucketProductionCompanies, EntityID: id, RawSumDelta: contribution * DefaultMultiplier})
	}

	return deltas
}
