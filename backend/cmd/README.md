# cmd

One subfolder per runnable program, each with `main.go` and a test. By Go convention `main.go` only wires things together; logic lives in `internal/`.

- `api/` – the hello-world API (`/healthz`, `/readyz`, `/version`).
- `auth/` – auth service skeleton: loads config, opens the Postgres pool, serves health probes plus `GET /api/auth/version`, and shuts down gracefully on SIGTERM.

Worth knowing: `signal.NotifyContext` cancels a `context` on SIGINT/SIGTERM (Kubernetes sends SIGTERM); `version` is a package variable overwritten at link time by the Dockerfile.
