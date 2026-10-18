# health

Probe handlers. `Checker` is a small **interface** (`Name`, `Check(ctx)`): anything that satisfies it (Postgres today, Redis/NATS later) can be added to `/readyz` without changing the handler. Checks run under a 2 s timeout, and error details are never returned to callers. Uses Go 1.22 method patterns (`"GET /healthz"`).
