# database

`Open` builds a pgx pool (max 5 connections, lifetimes so failovers are picked up). The pool connects lazily, so a service starts even while Postgres boots. `Checker` implements `health.Checker` with `Pool.Ping`. The test needs `TEST_DATABASE_URL` (CI provides a Postgres service container).
