# backend

The Go API service.

- `Dockerfile` – builds the container image.
- `go.mod` – Go module and dependencies.
- `.dockerignore` – files kept out of the image build.
- `cmd/` – program entry points.
- `config/` – loads settings from environment variables.
- `internal/` – the service's private code.
- `migrations/` – database schema changes (goose).
- `tests/` – integration tests.
