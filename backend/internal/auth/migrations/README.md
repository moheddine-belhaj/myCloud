# migrations

The auth schema, as [goose](https://github.com/pressly/goose) SQL migrations.

- `00001_init.sql` – creates `users`, `invites`, `sessions`. `-- +goose Up` / `-- +goose Down` mark apply and rollback sections.
- `00002_totp.sql` – `user_totp` and `recovery_codes` for two-factor authentication.
- `00003_mfa_challenges.sql` – `mfa_challenges`: half-finished logins (token hash, 5-minute expiry, attempt counter).
- `embed.go` – `//go:embed *.sql` bakes the files into the binary (`migrations.FS`), so the distroless image needs no extra files.
- `migrations_test.go` – applies the migrations on a throwaway database and checks the constraints.

## Rules

- Never edit a merged migration; add `00002_...` instead (zero-padded so order is lexical).
- Keep changes backward compatible (expand, then contract): during a rolling update old and new pods share the schema.
- The service applies them at startup via `database.Migrator` (background retry, advisory lock so only one replica migrates; `/readyz` fails until done).
