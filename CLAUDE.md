# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Movizius API is a Movie & TV Series tracking backend written in Go, deployed on Vercel using Go Serverless Functions. Features: authentication (Auth0), user profiles, watchlist, watch history, ratings, search, recommendations, TMDB synchronization.

## Commands

```bash
# Build all packages
go build ./...

# Run locally (auto-loads .env)
go run ./cmd/api

# Format check
gofmt -l .

# Vet
go vet ./...

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/health/...

# Run a single test
go test -run TestFunctionName ./internal/health/...

# Add a dependency
go get <module>@latest && go mod tidy
```

## Tech Stack

- Go 1.23 (installed), target 1.25+ when available
- Stdlib `net/http` with Go 1.22+ method-pattern routing (`"GET /path"`)
- MongoDB Atlas via `go.mongodb.org/mongo-driver v1` (database name: `moviedb`)
- Upstash Redis via HTTP REST API (not TCP — stateless for serverless)
- Auth0 for authentication (JWT/JWKS)
- Vercel Go Serverless Functions

## Vercel Deployment Model

**Critical constraint:** Vercel builds each `.go` file under `api/` as an independent serverless function. `api/index.go` exports `Handler(w, r)` and is the single entrypoint. `vercel.json` rewrites all paths (`/(.*) → /api`) so the internal `http.ServeMux` routes them.

Connections are initialized in `init()` in `api/index.go` — this runs once per cold start and is reused across warm invocations. Do NOT use `godotenv.Load()` there (Vercel sets env vars natively). `cmd/api/main.go` is for local dev and does load `.env`.

## Architecture

Feature-based modular layout. Each feature lives under `internal/<feature>/` and contains `handler.go`, `service.go`, `repository.go`, `model.go`, `dto.go`, `mapper.go`. Simple features (like `health`) omit layers they don't need.

**Dependency flow:** HTTP → Handler → Service → Repository → MongoDB. Never bypass layers.

### Dependency Injection

Infrastructure deps are passed top-down via `router.Deps`:

```go
// internal/shared/router/router.go
type Deps struct {
    DB    *mongo.Database
    Cache cache.Cache
}
func New(deps Deps) *http.ServeMux
```

Features receive their deps through constructors:
```go
health.NewHandler(health.NewService()).RegisterRoutes(mux)
// future: watchlist.NewHandler(watchlist.NewService(watchlist.NewRepository(deps.DB))).RegisterRoutes(mux)
```

### Adding a New Feature

1. Create `internal/<feature>/` with at minimum `handler.go`, `service.go`, `model.go`.
2. `handler.go`: `NewHandler(svc)` constructor + `RegisterRoutes(mux *http.ServeMux)` method. Use `response.Success` / `response.Error` for all responses.
3. `service.go`: `NewService(repo)` constructor, all methods take `ctx context.Context` first.
4. `repository.go`: `NewRepository(db *mongo.Database)` constructor, only MongoDB access here.
5. Wire it in `internal/shared/router/router.go` with one `RegisterRoutes` call.

### Shared Packages

- **`pkg/config`** — `config.Load()` reads required env vars (`MONGO_URI`, `UPSTASH_REDIS_REST_URL`, `UPSTASH_REDIS_REST_TOKEN`) and optional `PORT`. Returns error on missing required vars.
- **`pkg/database`** — `database.Connect(ctx, uri)` dials Atlas + pings; `database.DB(client, name)` returns a `*mongo.Database`.
- **`pkg/cache`** — `cache.Cache` interface (`Get/Set/Delete`). `cache.NewUpstash(url, token)` uses Upstash HTTP REST — no TCP connection. Each op is an HTTP request to `{url}/get/{key}`, `/set/{key}/{value}/ex/{secs}`, `/del/{key}` with `Authorization: Bearer {token}`.
- **`pkg/logger`** — `logger.New()` returns a `*slog.Logger` writing JSON to stderr.
- **`internal/shared/response`** — `response.Success(w, status, data)` and `response.Error(w, status, msg)`. Always use these; never hand-roll JSON envelopes.

## Authentication

Auth0 owns login/registration/token issuance. The API only validates JWTs.

- Identify users by `auth0_user_id` (the `sub` claim, e.g. `auth0|68234abcd1234`). Never use email as a primary identifier.
- Users are created lazily on first authenticated request: validate token → extract `sub` → upsert user record.
- All protected endpoints must validate: issuer (`iss`), audience (`aud`), expiration (`exp`), signature (Auth0 JWKS).
- Auth middleware (`internal/shared/middleware/auth.go`, to be implemented) reads `Authorization: Bearer`, validates, stores claims in request context.
- All watchlist/rating/history/recommendation operations must scope by `auth0_user_id` from the JWT — never from the request payload.

## Domain Rules

**Watchlist statuses:** `watching`, `completed`, `planned`, `paused`, `dropped`. Movies and TV share the same collection, distinguished by `media_type: "movie" | "tv"`.

**TMDB caching:** TMDB is the source of truth for movies/TV/genres/credits/images. Always check local cache first; call TMDB only when the record is missing, stale, or a refresh is requested.

**MongoDB indexes required on:** `user_id`, `media_id`, `status`, `updated_at`. Use projections when full documents aren't needed. No N+1 queries.

## Code Conventions

- Constructors for every type (`NewXxx`).
- Always pass `context.Context` as the first argument to service and repository methods.
- Wrap errors: `fmt.Errorf("failed to create watchlist item: %w", err)`.
- Structured logging via `pkg/logger` — no `fmt.Println`.
- Table-driven tests preferred.

## Documents
- Swagger for all api routes

## Sync data from 3rd party Rules
- Not update vote_average and vote_count because we use data from imdb.

## Response of movie, tv rules
- Use data from database as priority first. if not then use from tmdb.
