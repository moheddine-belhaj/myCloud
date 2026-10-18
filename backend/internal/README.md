# internal

Private code, split by responsibility. Go forbids importing `internal/` from outside this module.

- `health/` – `/healthz` (liveness, no dependency checks) and `/readyz` (readiness, runs registered `Checker`s).
- `database/` – pgx connection pool (small, lazy) and a `Checker` so `/readyz` pings Postgres.
- `httpserver/` – runs an `http.Server` and shuts it down gracefully when the context is cancelled.
- `auth/`, `user/`, `space/`, `file/`, `storage/` – placeholders for upcoming phases (`.gitkeep`).
