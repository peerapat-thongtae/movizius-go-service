# Movizius API

A Movie & TV Series tracking backend written in Go. It powers user watchlists, watch
history, search, discovery, and recommendations, backed by a catalog sourced from TMDB.

## Features

- **Movie & TV tracking** — track what you're watching with per-title states and
  episode progress.
- **Discovery & search** — discover, search, and get random movie/TV picks.
- **Catalog data** — rich movie and TV metadata (genres, credits, images).
- **Push notifications** — alerts for titles airing or releasing today.
- **API documentation** — Swagger reference served from the running service.

## Tech Stack

- Go (standard library `net/http`)
- MongoDB
- Redis (caching)
- JWT-based authentication
- External data providers: TMDB and TVMaze

## Getting Started

### Prerequisites

- Go 1.23+
- A MongoDB instance
- Redis (for caching)

### Configuration

The service is configured through environment variables. For local development, create a
`.env` file with the required values (database connection, cache, authentication, and
external API credentials). Refer to the configuration loader for the full list.

### Run Locally

```bash
# Start the API server (auto-loads .env)
go run ./cmd/api

# Server listens on http://localhost:8080 (Swagger at /api/swagger/)
```

## Development

```bash
go build ./...              # Build all packages
gofmt -l .                  # Format check
go vet ./...                # Vet
go test ./...               # Run all tests
go test ./internal/tv/...   # Test a single package
```

## License
Private project.
</content>
