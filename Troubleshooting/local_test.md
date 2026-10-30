# Local testing: auth service

Run the auth service on your laptop against a throwaway Postgres (`pg-test`), then test it with
`go test`, curl and Postman, including 2FA (M3).

> Everything here is local and disposable. Never use these passwords, keys or TOTP secrets in the cluster.

## Contents

- [Prerequisites](#prerequisites)
- [1. Start Postgres](#1-start-postgres)
- [2. Set the environment](#2-set-the-environment)
- [3. Run the tests](#3-run-the-tests)
- [4. Admin: check and create](#4-admin-check-and-create)
- [5. Run the server](#5-run-the-server)
- [6. Test with Postman](#6-test-with-postman)
- [7. Test 2FA with your phone](#7-test-2fa-with-your-phone)
- [8. Reset and clean up](#8-reset-and-clean-up)
- [Troubleshooting](#troubleshooting)

## Prerequisites

| Tool | Used for |
|---|---|
| `go` (1.26+) | building and testing the backend |
| `docker` | the local Postgres `pg-test` |
| `openssl` | the local TOTP key |
| Postman | the API collection `mycloud-auth.postman_collection.json` |
| `oathtool`, `qrencode` (optional) | 2FA codes and QR codes in the terminal: `sudo apt install oathtool qrencode` |

All commands run from `backend/` unless noted.

## 1. Start Postgres

`pg-test` listens on **55432**, so it never collides with another Postgres on 5432.

```bash
docker ps -a --filter name=pg-test        # listed but stopped? docker start pg-test
# not there yet:
docker run -d --name pg-test -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app \
  -e POSTGRES_DB=app -p 55432:5432 postgres:18
sleep 3
```

## 2. Set the environment

```bash
cd backend
export TEST_DATABASE_URL='postgres://app:app@localhost:55432/app?sslmode=disable'
export DATABASE_URL="$TEST_DATABASE_URL"
# local TOTP key: created once, kept in a file so it stays the same across restarts
[ -f ~/.mycloud-local-totp-key ] || (umask 077; openssl rand -base64 32 > ~/.mycloud-local-totp-key)
export TOTP_KEY=$(cat ~/.mycloud-local-totp-key)
```

| Variable | Used by | Note |
|---|---|---|
| `TEST_DATABASE_URL` | `go test` | Each test creates and drops its own database on this server |
| `DATABASE_URL` | the server and the CLI | Uses the `app` database |
| `TOTP_KEY` | the server | Must not change while 2FA users exist, or their secrets can't be decrypted |

Set these in every new terminal.

## 3. Run the tests

```bash
gofmt -l .                      
go vet ./...
go test -race ./...             # all ok
```

The database tests must **run**, not skip (a skip means `TEST_DATABASE_URL` is not set):

```bash
go test -v ./internal/auth ./internal/auth/migrations | grep -E -- '--- (PASS|SKIP|FAIL)' | sort | uniq -c | sort -rn | head
go test -v -run 'TOTP|Recovery|LoginWithTOTP|LoginTOTP|ResetTOTP|AdminNeeds2FA' ./internal/auth | grep -E '^(--- |ok)'
```

After changing a query or a migration, regenerate the sqlc code (CI runs `sqlc diff` and fails if it's stale):

```bash
docker run --rm -v "$PWD":/src -w /src sqlc/sqlc:1.31.1 generate
```

## 4. Admin: check and create

Is there an admin?

```bash
docker exec pg-test psql -U app -d app -c "select email, role, created_at from users where role = 'admin'"
```

| Output | Meaning |
|---|---|
| one or more rows | an admin exists; use it if you know its password |
| `(0 rows)` | no admin yet |
| `relation "users" does not exist` | fresh database: no admin yet |

Create one. The CLI applies the migrations first, then creates the admin. The password goes through stdin,
so it never shows up in the process list or shell history:

```bash
read -rsp 'Admin password (12+ chars): ' PW; echo
printf '%s\n' "$PW" | go run ./cmd/auth create-admin --email admin@example.com
unset PW
```

Expect `created admin admin@example.com (id …)`. There is no password reset: if you forget it, create a second
admin with another email (or start over, step 8).

Other CLI commands:

```bash
go run ./cmd/auth create-invite --ttl 24h              # prints an invite code
go run ./cmd/auth reset-2fa --email admin@example.com  # removes 2FA, ends all sessions
go run ./cmd/auth help
```

## 5. Run the server

In a terminal with the variables from step 2:

```bash
go run ./cmd/auth
```

The log shows `listening … "totp_key_id":1` and the applied migrations (`00001_init.sql`, `00002_totp.sql`,
`00003_mfa_challenges.sql`).

In another terminal:

```bash
curl -s localhost:8080/readyz; echo             # {"status":"ok","checks":{"migrations":"ok","postgres":"ok"}}
curl -s localhost:8080/api/auth/version; echo   # {"service":"auth","version":"dev"}
```

## 6. Test with Postman

1. Import `mycloud-auth.postman_collection.json`.
2. Collection → Variables:

   | Variable | Value |
   |---|---|
   | `baseUrl` | `http://localhost:8080` |
   | `adminEmail` | `admin@example.com` |
   | `adminPassword` | the password from step 4 |

3. Run the collection (Run collection → Run).

What happens:

| Folder | Tests |
|---|---|
| 1. Health | `/readyz`, `/version` |
| 2. Admin creates an invite | admin login; **first run only:** turns on the admin's 2FA and saves the secret in `adminTotpSecret`; later runs answer the code step; creates invites |
| 3. Register | a new user registers with the invite (a fresh `user…@example.com` every run) |
| 4. User session | login, `/me`, forbidden invite, logout |
| 5. Without a session | 401s and the cross-site POST (403) |
| 6. Validation errors | bad email, short password, extra fields, broken JSON, wrong content type |
| 7. Two-factor authentication | setup, enable, login with a wrong code / recovery code / app code, disable |
| 8. Admin logout | |

Postman computes the TOTP codes itself (pre-request scripts), so no phone is needed. Some requests wait up to
**30 seconds** on purpose: a code works only once, so they wait for the next one.

The session cookie is `Secure`, so Postman won't resend it over `http://localhost` by itself. The collection
saves the tokens in variables and sends them in a `Cookie` header.

## 7. Test 2FA with your phone

TOTP works offline: the app and the server compute the same 6-digit code from the shared secret.
Use a Postman request per call. Postman doesn't resend the `Secure` cookie over `http://localhost`, so add it as a
`Cookie` header yourself (value from the login response's `Set-Cookie`).

**1. Log in**

`POST http://localhost:8080/api/auth/login`, body (raw JSON):

```json
{ "email": "user@example.com", "password": "your-password" }
```

Copy `__Host-session=<token>` from the response `Set-Cookie`. Add it as header `Cookie: __Host-session=<token>`
on the next requests.

**2. Start setup**

`POST http://localhost:8080/api/auth/2fa/setup`, body `{}`. The response has `secret` and `uri`.

**3. QR code**: render the URI in your terminal and scan it.

```bash
sudo apt install qrencode
qrencode -t ansiutf8 'otpauth://totp/MyCloud%20dev:...&secret=...'   # paste your full uri
```

Or in the app: "Enter a setup key" with `secret`.

**4. Enable**

`POST http://localhost:8080/api/auth/2fa/enable`

```json
{ "code": "123456" }
```

Response: 10 `recoveryCodes`. Check: `GET http://localhost:8080/api/auth/me` returns `"totpEnabled": true`.

**5. Log in again (two steps)**

`POST http://localhost:8080/api/auth/login` with the same body as step 1. The response sets `__Host-mfa=<token>`.

`POST http://localhost:8080/api/auth/login/totp`, header `Cookie: __Host-mfa=<token>`:

```json
{ "code": "123456" }
```

Expect `204`, a new `__Host-session` and a cleared `__Host-mfa`. A recovery code (`xxxx-xxxx-xxxx-xxxx`) works
once in place of the app code.

**6. Disable**

`POST http://localhost:8080/api/auth/2fa/disable`, header `Cookie: __Host-session=<token>`:

```json
{ "password": "your-password", "code": "123456" }
```

Expect `204`. A code works only once: wait for the next one between steps.

Optional: the terminal formula must match the phone (same 6 digits within 30 s):

```bash
oathtool --totp -b <secret from setup>
```

Expected answers:

| Request | Answer |
|---|---|
| wrong, expired or reused code | `400 invalid_code` |
| 6th wrong code on one login | `401 mfa_expired` (log in with the password again) |
| no `__Host-mfa` cookie | `401 mfa_expired` |
| disable with a wrong password | `403 wrong_password` |
| setup while 2FA is on | `409 totp_already_enabled` |
| enable before setup | `409 totp_not_set_up` |
| invite as an admin without 2FA | `403 totp_required` |

Look inside the database (only ciphertext and hashes):

```bash
docker exec pg-test psql -U app -d app -c \
  "select u.email, left(encode(t.secret_enc,'hex'),4) as header, length(t.secret_enc), t.enabled_at is not null as enabled
   from user_totp t join users u on u.id = t.user_id"
docker exec pg-test psql -U app -d app -c \
  "select count(*) filter (where used_at is null) as unused, count(*) as total from recovery_codes"
```

`header` is `0101` (format 1, key 1) and `length` is 50 bytes (2 + 12 nonce + 20 secret + 16 tag).

## 8. Reset and clean up

| Goal | Command |
|---|---|
| Admin has 2FA but Postman lost the secret | `go run ./cmd/auth reset-2fa --email admin@example.com` |
| Empty all tables, keep the container | `docker exec pg-test psql -U app -d app -c 'drop schema public cascade; create schema public;'` then step 4 |
| Remove everything | `docker rm -f pg-test` (and `rm ~/.mycloud-local-totp-key` if you want a new key) |
| Stop the server | Ctrl-C (it shuts down gracefully) |

After emptying the database, also clear `adminTotpSecret` in Postman (Collection → Variables).

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `password authentication failed for user "app"` on `localhost:5432` | Another Postgres answers on 5432. Use `pg-test` on **55432** (steps 1 and 2) |
| Database tests show `SKIP` | `TEST_DATABASE_URL` is not set in this terminal |
| `connection refused` on 55432 | `pg-test` is stopped: `docker start pg-test` |
| Server exits: `TOTP_KEY is required` / `must be 32 bytes` | Step 2 not done in this terminal |
| Server exits: `DATABASE_URL is required` | Step 2 not done in this terminal |
| 2FA requests return 500, log says `open totp secret` | `TOTP_KEY` changed since 2FA was set up. Use the key from `~/.mycloud-local-totp-key`, or `reset-2fa` the user |
| Postman stops at "Login (admin): 2FA code": `adminTotpSecret is empty` | The admin has 2FA, Postman lost the secret: `reset-2fa` the admin |
| Every code is `invalid_code` | Clock drift (±30 s allowed): check the phone's time. Or the code was already used: wait for the next one |
| `bind: address already in use` on :8080 | Another server runs on 8080: stop it, or `ADDR=:8090 go run ./cmd/auth` and change `baseUrl` |
| CI `sqlc diff` fails | Regenerate with sqlc 1.31.1 (step 3) and commit `internal/auth/store` |