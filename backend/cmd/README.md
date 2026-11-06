# cmd

One subfolder per runnable program, each with `main.go` and a test. By Go convention `main.go` only wires things together; logic lives in `internal/`.

- `api/` – the hello-world API (`/healthz`, `/readyz`, `/version`).
- `auth/` – the auth service. `serve` (default) loads config, opens the Postgres pool, runs migrations, serves health probes, `GET /api/auth/version` and the auth routes (register, login, 2FA login step, logout, `/me`, admin invites, 2FA setup/enable/disable, recovery codes), runs an hourly cleanup of expired sessions and 2FA challenges, and shuts down gracefully on SIGTERM. `cli.go` adds admin commands run inside the pod: `create-admin --email EMAIL` (password on stdin, so it never shows in the process list) `create-invite --ttl 72h` and `reset-2fa --email EMAIL` (removes 2FA and ends all sessions for a user who lost both phone and recovery codes; verify identity out of band first).

Worth knowing: `signal.NotifyContext` cancels a `context` on SIGINT/SIGTERM (Kubernetes sends SIGTERM); `version` is a package variable overwritten at link time by the Dockerfile.
