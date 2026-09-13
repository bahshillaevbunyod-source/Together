# Together API

Minimal, standard-library-only Go backend for Together.

## Run

```bash
cd backend
go run ./cmd/api
# GET http://localhost:8080/health -> {"status":"ok"}
```

Configuration comes from the environment (see `.env.example`):

- `PORT` — HTTP port (fallback `8080`)

## Test

```bash
cd backend
go test ./...
```

## Layout

```
cmd/api/            process entrypoint (config load, server start, graceful shutdown)
internal/config/    environment configuration
internal/server/    HTTP server, routing, handlers
```

Routing is centralized in `internal/server/registerRoutes`, so future
subsystems each get their own `internal/<name>` package and route group:

- `internal/auth` — auth / JWT
- `internal/db` (or `internal/store`) — PostgreSQL
- `internal/ws` — WebSocket
- `internal/translation` — translation service

None of these are included yet.
