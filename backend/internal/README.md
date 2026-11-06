# internal

Private code, split by responsibility. Go forbids importing `internal/` from outside this module.

- `health/` – `/healthz` (liveness) and `/readyz` (readiness, runs registered `Checker`s).
- `database/` – pgx pool, health check and the goose `Migrator`.
- `httpserver/` – runs an `http.Server` with graceful shutdown.
- `auth/` – auth service: HTTP handlers, business logic (register, login, sessions, invites, TOTP 2FA with recovery codes and a two-step login), plus migrations, SQL queries, sqlc-generated `store/`, `password/` (argon2id), `token/`, `keyring/` (encrypts TOTP seeds) and `totp/` (RFC 6238 codes).
- `testdb/` – helper giving each test its own throwaway Postgres database.
- `user/`, `space/`, `file/`, `storage/` – placeholders for upcoming phases (`.gitkeep`).
