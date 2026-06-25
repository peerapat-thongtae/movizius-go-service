# CLAUDE.md

## Project Overview

Movizius API is a Movie & TV Series tracking backend written in Go.

Core features:

* Authentication
* User profiles
* Watchlist management
* Watch history
* Ratings
* Search
* Recommendations
* TMDB synchronization

This API is deployed on Vercel using Go Serverless Functions.

---

## Tech Stack

* Go 1.25+
* Gin
* MongoDB Atlas
* JWT Authentication
* Vercel
* TMDB API

---

## Architecture

Use feature-based modular architecture.

Structure:

```text
cmd/
└── api/

internal/
├── auth/
├── user/
├── movie/
├── tv/
├── watchlist/
├── history/
├── rating/
├── search/
├── recommendation/
└── shared/

pkg/
├── database/
├── jwt/
├── logger/
└── cache/
```

Each feature should contain:

```text
feature/
├── handler.go
├── service.go
├── repository.go
├── model.go
├── dto.go
└── mapper.go
```

---

## Layer Responsibilities

### Handler

Responsibilities:

* Parse request
* Validate request
* Call service
* Return HTTP response

Handlers must not:

* Access MongoDB directly
* Contain business logic

### Service

Responsibilities:

* Business logic
* Validation rules
* Coordination between repositories

Services must not:

* Know HTTP details
* Build MongoDB queries directly

### Repository

Responsibilities:

* MongoDB access
* Query building
* Aggregation pipelines

Repositories must not:

* Contain business rules

---

## Dependency Flow

```text
HTTP
 ↓
Handler
 ↓
Service
 ↓
Repository
 ↓
MongoDB
```

Never bypass layers.

---

## Code Style

### Constructors

Always use constructors.

Example:

```go
func NewWatchlistService(
    repo WatchlistRepository,
) *WatchlistService {
    return &WatchlistService{
        repo: repo,
    }
}
```

### Context

Always pass context.Context.

Example:

```go
func (s *WatchlistService) Add(
    ctx context.Context,
    req AddWatchlistRequest,
) error
```

### Errors

Return errors instead of panic.

Use wrapped errors:

```go
return fmt.Errorf(
    "failed to create watchlist item: %w",
    err,
)
```

### Logging

Use structured logging.

Never use fmt.Println in production code.

---

## MongoDB Rules

Use indexes when querying by:

* user_id
* media_id
* status
* updated_at

Prefer projections when full documents are not required.

Avoid N+1 queries.

Use aggregation only when necessary.

---

## API Design

REST conventions:

```text
GET    /watchlist
POST   /watchlist
PATCH  /watchlist/:id
DELETE /watchlist/:id
```

Response format:

```json
{
  "success": true,
  "data": {}
}
```

Error format:

```json
{
  "success": false,
  "message": "resource not found"
}
```

---

## Watchlist Domain

Supported statuses:

```text
watching
completed
planned
paused
dropped
```

Movie and TV records share the same watchlist collection.

Example:

```json
{
  "user_id": "123",
  "media_id": 550,
  "media_type": "movie",
  "status": "watching"
}
```

---

## TMDB Integration

TMDB is the source of truth for:

* Movies
* TV Series
* Genres
* Credits
* Images

Local database stores:

* Cached metadata
* User-specific data
* Watchlist data
* Ratings
* History

Prefer fetching from local cache first.

Only call TMDB when:

* Record is missing
* Cache is stale
* Manual refresh requested

---

## Vercel Deployment

Requirements:

* Compatible with Vercel Go Runtime
* Stateless architecture
* No local filesystem persistence
* No background jobs
* No cron dependencies inside API routes

Use environment variables only.

Never store secrets in source code.

---

## Performance Guidelines

Prefer:

* Batch database operations
* Pagination
* Database indexes
* Projection queries

Avoid:

* Loading entire collections
* Unbounded queries
* Large aggregation pipelines on hot paths

---

## Testing

Generate tests for:

* Services
* Repositories
* Critical business logic

Prefer table-driven tests.

Example:

```go
func TestWatchlistStatusValidation(t *testing.T)
```

---

## When Generating Code

Claude should:

1. Follow existing architecture.
2. Reuse existing services whenever possible.
3. Avoid creating duplicate abstractions.
4. Keep implementations simple.
5. Prefer readability over cleverness.
6. Preserve backward compatibility unless explicitly requested.
7. Update tests when changing business logic.
8. Run gofmt-compatible formatting.
9. Never introduce unnecessary frameworks.
10. Minimize dependencies.

```
## Authentication

Authentication is managed entirely by Auth0.

The backend API does not handle:

* Username/password login
* Password reset
* User registration
* Session management
* Token issuance

These responsibilities belong to Auth0.

### Authentication Flow

```text
Flutter App
    ↓
Auth0 Login
    ↓
Access Token (JWT)
    ↓
Go API
    ↓
JWT Validation
    ↓
Authenticated Request
```

### User Identity

Use the Auth0 user identifier (`sub`) as the primary user identifier.

Example:

```json
{
  "sub": "auth0|68234abcd1234",
  "email": "user@example.com"
}
```

Store `sub` as `auth0_user_id`.

Example:

```json
{
  "auth0_user_id": "auth0|68234abcd1234"
}
```

Never use email as the primary identifier.

### Backend Responsibilities

The API is responsible for:

* Verifying JWT access tokens
* Extracting Auth0 claims
* Creating user records on first login
* Authorizing requests
* Managing user-specific data

The API is NOT responsible for:

* Login UI
* Password management
* OAuth flows
* Social login integration

### JWT Validation

All protected endpoints must validate:

* Issuer (`iss`)
* Audience (`aud`)
* Expiration (`exp`)
* Signature

Use Auth0 JWKS for signature verification.

### Auth Middleware

Authentication should be implemented as middleware.

Example:

```text
middleware/
└── auth.go
```

The middleware should:

1. Read the Authorization header.
2. Validate the Bearer token.
3. Extract Auth0 claims.
4. Store user information in request context.
5. Reject unauthorized requests.

### User Creation Strategy

Users are created lazily.

On the first authenticated request:

1. Validate Auth0 token.
2. Extract `sub`.
3. Check whether user exists.
4. Create user if not found.

Example:

```json
{
  "auth0_user_id": "auth0|68234abcd1234",
  "display_name": "",
  "created_at": "2026-06-25T00:00:00Z"
}
```

### Database Rules

Users collection should contain:

```json
{
  "_id": "...",
  "auth0_user_id": "auth0|68234abcd1234",
  "display_name": "",
  "avatar_url": "",
  "created_at": "",
  "updated_at": ""
}
```

Create a unique index on:

```text
auth0_user_id
```

### Authorization

All watchlist, rating, history, and recommendation operations must be scoped by the authenticated user's `auth0_user_id`.

Never trust user identifiers sent in request payloads.

Always obtain the user identity from the validated JWT.

```
```

```
