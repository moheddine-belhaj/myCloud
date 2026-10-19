# internal

Private code, split by responsibility. Go forbids importing `internal/` from outside this module.

- `health/` – `/healthz` (liveness) and `/readyz` (readiness, runs registered `Checker`s).
- `database/` – pgx pool, health check and the goose `Migrator`.
- `httpserver/` – runs an `http.Server` with graceful shutdown.
- `auth/` – auth service data layer: migrations, SQL queries, sqlc-generated `store/`.
- `testdb/` – helper giving each test its own throwaway Postgres database.
- `user/`, `space/`, `file/`, `storage/` – placeholders for upcoming phases (`.gitkeep`).
