# 08 · M2: Authentication logic

Goal: invite-only registration, password login and server-side sessions.

## PR 1: schema and migrations
1. Write the schema as goose migrations (`users`, `invites`, `sessions`), embedded in the binary.
2. Generate type-safe queries with sqlc (`internal/auth/store`).
3. Run migrations on start-up.
4. Add `internal/testdb` for tests against a real Postgres.

## PR 2: passwords, invites, registration
1. Hash passwords with argon2id (19 MiB, t=2, p=1, PHC format), at most 4 hashes at a time.
2. Generate random tokens and store only their SHA-256 hash (`internal/auth/token`).
3. Create invites (single-use, expiring) and `POST /api/auth/register` with an invite code.
4. Add a CLI to the auth binary: `create-admin` (password on stdin) and `create-invite`.

## PR 3: sessions and login
1. `POST /api/auth/login` with a dummy hash for unknown emails (no timing leak).
2. Session cookie `__Host-session`: HttpOnly, Secure, SameSite=Strict, 7 days; only the token's hash is stored.
3. `POST /api/auth/logout` and `GET /api/auth/me`.
4. `POST /api/auth/invites` for admins only.
5. CSRF protection with Go's `http.CrossOriginProtection`.
6. Background cleanup of expired sessions.
7. Test documentation and a Postman collection for the whole flow.

Result: an admin invites a friend, the friend registers, logs in and out, all through the API.
