# backend

Go module `github.com/moheddine-belhaj/mycloud/backend`. Several small services share one module and one image recipe.

- `cmd/` – one folder per program (`api`, `auth`).
- `config/` – settings from environment variables.
- `internal/` – private packages: health, database (pool + migrator), httpserver, auth (migrations/queries/sqlc store), testdb.
- `migrations/` – unused placeholder; migrations live in `internal/<service>/migrations`.
- `sqlc.yaml` – sqlc config: generates type-safe Go from SQL.
- `tests/` – integration tests, empty for now.
- `Dockerfile` – **multi-stage build**: compiles a static binary (`CGO_ENABLED=0`), copies it into a *distroless nonroot* image. The build arg `SERVICE` picks the program (`./cmd/${SERVICE}`), `VERSION` is injected with `-ldflags -X main.version=...`.
- `.dockerignore` – files kept out of the build context.
- `go.mod` / `go.sum` – dependencies (pgx for Postgres).

```bash
go test -race ./...
DATABASE_URL=postgres://... go run ./cmd/auth
docker build --build-arg SERVICE=auth -t mycloud-auth:local .
```
