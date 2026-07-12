# Recommendation Profile System — Requirements Spec

## 1. Overview

Build a service that computes and maintains a **per-user recommendation profile** from watch history (movies & TV), based on TMDB metadata. The profile is a set of weighted scores per entity type (genre, keyword, actor, director, etc.), used later to rank candidate titles for recommendations.

Two-phase pipeline:
1. **Event ingestion** — every watch event produces a signed weight (`recencyWeight × completion × rewatchBonus × ratingSignal`).
2. **Aggregation** — weights are accumulated per entity (genre/keyword/actor/director/collection/company/network/creator), then normalized into a `-100..100` score.

---

## 2. Data Sources

### 2.1 `watch_history` (append-only event log)

| Field | Type | Notes |
|---|---|---|
| `user_id` | string | |
| `media_type` | enum `movie` \| `tv` | |
| `movie_id` / `tv_id` | int | TMDB id |
| `watched_at` | datetime | |
| `rating` | int, nullable (1-5) | explicit signal if present |
| `completion_pct` | float (0-1) | implicit signal, used when `rating` is null |
| `rewatch_count` | int | default 0 |

### 2.2 `movies` / `tv_shows` (TMDB metadata, cached locally)

Movie fields needed: `genres[]`, `keywords[]`, `director_id`, `cast_ids[]` (ordered by billing), `collection_id`, `production_company_ids[]`.

TV fields needed: `genres[]`, `keywords[]`, `creator_ids[]`, `cast_ids[]`, `network_ids[]`.

---

## 3. Event Weight Calculation

For each watch event, compute:

```
recencyWeight = exp(-ln(2) * daysSince / HALF_LIFE_DAYS)   // HALF_LIFE_DAYS = 90 (configurable)

ratingSignal =
    (rating - 3) / 2            if rating present   // maps 1..5 -> -1..+1
    (completion_pct - 0.5) * 2  otherwise            // maps 0..1 -> -1..+1

rewatchBonus = 1 + ln(1 + rewatch_count) * 0.3

magnitude = recencyWeight * completion_pct * rewatchBonus

contribution = magnitude * ratingSignal   // signed value, can be negative
```

`ratingSignal` is what allows negative preference (e.g. disliked genres end up with negative scores).

---

## 4. Aggregation Per Entity

For every watch event, distribute `contribution` into every entity bucket the title belongs to:

| Entity bucket | Source field | Extra weight multiplier |
|---|---|---|
| genres | `genres[]` | 1.0 |
| keywords | `keywords[]` | 1.0 |
| directors (movie) / creators (tv) | `director_id` / `creator_ids[]` | 1.2 |
| actors | `cast_ids[]` (top 5 only) | 1.2 for index 0 (lead), 1.0 otherwise |
| collections | `collection_id` | 1.0 |
| productionCompanies | `production_company_ids[]` | 1.0 |
| networks (tv only) | `network_ids[]` | 1.0 |

For each `(bucket, entity_id)` pair, maintain running totals:

```
rawSum += contribution * multiplier
count  += 1
```

These raw totals (`rawSum`, `count`) must be **persisted**, not just the final score — they are required for incremental updates (see §6).

---

## 5. Score Normalization

Convert raw accumulation into the public-facing `score` (integer, -100..100):

```
avgSignal = rawSum / count
squashed  = tanh(avgSignal)        // smooth bound to [-1, 1], no hard clipping
score     = round(squashed * 100)
```

`count` is kept alongside `score` in the output (confidence/frequency indicator).

---

## 6. Update Strategy

- **Do not recompute from full history on every event.** Store `rawSum` and `count` per entity in the profile document itself.
- On a new watch event: `rawSum += contribution`, `count += 1` for every affected entity (incremental `$inc`-style update), then re-run normalization only for the touched entities.
- Trigger: background job/queue consumer on new `watch_history` insert, OR nightly batch recompute as a fallback/reconciliation job.
- Full recompute from raw `watch_history` should still be supported as an admin/repair operation (e.g. after changing `HALF_LIFE_DAYS` or scoring formula version).

---

## 7. Output Schema (`recommendationProfile`)

```json
{
  "recommendationProfile": {
    "version": 1,
    "movie": {
      "genres":              { "<genre_id>": { "score": -100..100, "count": int, "rawSum": float } },
      "keywords":             { "<keyword_id>": { "score": ..., "count": ..., "rawSum": ... } },
      "actors":                { "<actor_id>": { ... } },
      "directors":             { "<director_id>": { ... } },
      "collections":           { "<collection_id>": { ... } },
      "productionCompanies":   { "<company_id>": { ... } },
      "watchedIds": [int, ...],
      "embedding": [float, ...]
    },
    "tv": {
      "genres": { ... },
      "keywords": { ... },
      "actors": { ... },
      "creators": { ... },
      "networks": { ... },
      "watchedIds": [int, ...],
      "embedding": [float, ...]
    },
    "recentContext": {
      "movie": { "genreIds": [int, ...] },
      "tv": { "genreIds": [int, ...] },
      "windowDays": 14
    },
    "meta": {
      "totalMovieWatched": int,
      "totalTvWatched": int,
      "decayHalfLifeDays": 90,
      "sourceEventCount": int,
      "scoringVersion": 1
    },
    "updatedAt": "ISO8601 datetime"
  }
}
```

**Note:** `rawSum` is internal (needed for incremental recompute) — decide whether to expose it in the API response or keep it DB-only and strip it before returning to clients.

---

## 8. Non-functional Requirements

- **Entity bucket pruning**: periodically drop entities with low `count` and near-zero `score` (e.g. `count < 2 && |score| < 10`) to prevent unbounded document growth. Cap each bucket at top ~50-100 entities by `|score| * log(count)` or similar.
- **Config values must be tunable** without redeploy if possible: `HALF_LIFE_DAYS`, rewatch bonus coefficient, lead-actor multiplier, director/creator multiplier, pruning thresholds.
- **Idempotency**: reprocessing the same `watch_history` event twice must not double-count (use event id / upsert semantics, or dedupe before aggregation).
- **Versioning**: `scoringVersion` / `recommendationProfile.version` must bump whenever the formula changes, to allow safe migration/recompute of existing profiles.
- **Exclusion list**: `watchedIds` must be kept up to date and used to filter out already-seen titles at recommendation query time.

---

## 9. Suggested Go Package Boundaries

- `event/` — event weight calculation (§3): pure functions, easily unit-testable with table-driven tests.
- `aggregate/` — entity accumulation logic (§4): takes a watch event + movie/tv metadata, returns bucket deltas.
- `normalize/` — score normalization (§5): pure function `(rawSum, count) -> score`.
- `profile/` — profile document model, incremental update logic (§6), pruning (§8).
- `worker/` — queue consumer / cron job wiring, idempotency handling.

Keep §3–§5 as pure, side-effect-free functions — this makes unit testing the scoring math trivial and decouples it from MongoDB/queue infrastructure.