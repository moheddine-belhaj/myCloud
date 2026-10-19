# database

- `database.go` – `Open` builds a pgx pool (max 5 connections, lifetimes so failovers are picked up). The pool connects lazily, so a service starts even while Postgres boots. `Checker` implements `health.Checker` with `Pool.Ping`.
- `migrate.go` – `Migrator` applies a service's embedded goose migrations. `RunWithRetry` loops with exponential backoff (1 s → 30 s) in a goroutine; a Postgres **advisory lock** makes concurrent replicas safe. It also implements `health.Checker`: `/readyz` reports `migrations: fail` until the schema is applied.
- Tests need `TEST_DATABASE_URL` (CI provides a Postgres service container).
