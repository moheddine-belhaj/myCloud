# config

`config.go` reads settings from environment variables: `ADDR` (default `:8080`), `SHUTDOWN_TIMEOUT` (duration like `20s`) and `DATABASE_URL` (required, **fail fast** if missing). The Helm chart builds `DATABASE_URL` from a Secret, so the password never lives in Git in clear text.
