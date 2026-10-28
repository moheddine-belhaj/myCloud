# 01 · Project bootstrap

Goal: a repository with a clear architecture and a first Go service.

1. Create the repository `myCloud` with `.gitignore` and `README.md`.
2. Write the architecture and build plan (`docs/MyCloudArchitecture&BuildPlan.pdf`).
3. Record decisions as ADRs: `0001-record-architecture-decisions.md`, `0002-initial-stack.md`.
4. Lay out the Go module in `backend/` (`cmd/`, `config/`, `internal/`, `migrations/`, `tests/`).
5. Load configuration from environment variables (`backend/config`).
6. Build the API with `net/http` and `log/slog`: `/healthz`, `/readyz` and `/version` endpoints.
7. Work on the `dev_BE` branch and merge to `main` through pull requests.

Result: a minimal Go API with health probes and documented architecture decisions.
