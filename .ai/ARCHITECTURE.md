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
