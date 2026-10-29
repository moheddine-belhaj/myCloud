# auth service: test & verification commands

Commands to check that **M1** (Helm foundation + auth skeleton + own database) and
**M2** (auth logic) work: locally, on your PC before committing, and in the cluster.

Suggested location in the repo: `deploy/services/auth/README.md`

> Run cluster commands from your PC (kubectl reaches the cluster over Tailscale).
> Replace `mycloud-dev` with `mycloud-qa` or `mycloud-prod` to check other environments.

---

## Contents

- [Prerequisites](#prerequisites)
- [M1: before committing](#m1-before-committing)
- [M1: in the cluster](#m1-in-the-cluster)
- [M2 PR 1: schema & migrations (local)](#m2-pr-1-schema--migrations-local)
- [M2 PR 1: in the cluster](#m2-pr-1-in-the-cluster)
- [M3 PR 1: TOTP encryption key](#m3-pr-1-totp-encryption-key)
- [M3 PR 2: 2FA setup, enable, disable](#m3-pr-2-2fa-setup-enable-disable)
- [M3 PR 3: login with 2FA](#m3-pr-3-login-with-2fa)
- [Connect to the database by hand](#connect-to-the-database-by-hand)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

| Tool | Used for |
|---|---|
| `kubectl` | everything in the cluster |
| `vault` CLI, logged in (`vault login -method=userpass username=moheddine`) | reading secrets in Vault (compare hashes, don't print values) |
| `yq` | reading YAML |
| `helm` | rendering the chart (`sudo snap install helm --classic`) |
| `docker` | local Postgres for tests, running sqlc |
| `go` | building and testing the backend |

---

## M1: before committing

### `auth-db` from Vault (one per environment)

The credentials live in Vault at `secret/mycloud/<env>/auth-db`. Git only has an `ExternalSecret`
per overlay, so there is nothing to encrypt before committing.

Each overlay points at its own Vault path. Expect `key: mycloud/<env>/auth-db` twice per environment:
```bash
grep -n 'key:' deploy/apps/postgres/overlays/*/external-secret.yaml
```

No plain Secret anywhere in `deploy/` (CI runs the same check). Expect no output:
```bash
grep -rlE '^kind:[[:space:]]*Secret[[:space:]]*$' deploy
```

The postgres overlays render. Expect `Cluster/db`, `DatabaseRole/auth`, `Database/auth`,
`ExternalSecret/auth-db` for each environment:
```bash
for e in dev qa prod; do
  echo "== $e"; kubectl kustomize deploy/apps/postgres/overlays/$e | yq '.kind + "/" + .metadata.name'
done
```

Vault has the values (on your PC, logged in). Expect `auth` three times and three **different** hashes:
```bash
for e in dev qa prod; do
  vault kv get -field=username secret/mycloud/$e/auth-db; echo
  vault kv get -field=password secret/mycloud/$e/auth-db | sha256sum
done
```

### Helm chart

Lint the chart and render the Ingress for dev. Expect `0 chart(s) failed`, then
`host: cloud-dev.mohedine.dev`, `path: /api/auth`:
```bash
helm lint deploy/charts/service -f deploy/services/auth/values.yaml -f deploy/services/auth/dev/values.yaml
helm template auth deploy/charts/service -n mycloud-dev \
  -f deploy/services/auth/values.yaml -f deploy/services/auth/dev/values.yaml | yq 'select(.kind=="Ingress")'
```

Replicas and host per environment. Expect `1 cloud-dev…`, `1 cloud-qa…`, `2 cloud.mohedine.dev`:
```bash
for e in dev qa prod; do
  echo "== $e"
  helm template auth deploy/charts/service -n mycloud-$e \
    -f deploy/services/auth/values.yaml -f deploy/services/auth/$e/values.yaml \
  | yq 'select(.kind=="Deployment").spec.replicas, select(.kind=="Ingress").spec.rules[0].host'
done
```

> VS Code may show `Unexpected scalar token` in `templates/*.yaml`. That's the YAML
> extension reading Go templates; trust `helm lint` / `helm template` instead.

---

## M1: in the cluster

### Argo CD

The services ApplicationSet is applied (one-time, manual). Expect `auth-dev`, `auth-qa`, `auth-prod`:
```bash
kubectl -n argocd get applications | grep -E 'auth|postgres'
```
If the auth apps are missing:
```bash
git checkout main && git pull
kubectl apply -f deploy/argocd/applicationset-services.yaml
```

Details of one app (sync status, health, errors):
```bash
kubectl -n argocd get app auth-dev postgres-dev
kubectl -n argocd get app postgres-dev -o jsonpath='{.status.conditions}'; echo
```

### Database role and database (CNPG)

ESO synced `auth-db` in every environment. Expect `SecretSynced  True` three times:
```bash
for e in dev qa prod; do kubectl -n mycloud-$e get externalsecret auth-db; done
```

The Secret in the cluster matches Vault (hashes only). Expect `OK` three times:
```bash
for e in dev qa prod; do
  a=$(vault kv get -field=password secret/mycloud/$e/auth-db | sha256sum)
  b=$(kubectl -n mycloud-$e get secret auth-db -o jsonpath='{.data.password}' | base64 -d | sha256sum)
  [ "$a" = "$b" ] && echo "$e OK" || echo "$e DIFFERENT"
done
```

The `auth-db` Secret has the right type and label. Expect `kubernetes.io/basic-auth  reload=true` three times:
```bash
for e in dev qa prod; do
  kubectl -n mycloud-$e get secret auth-db -o jsonpath='{.type}{"  reload="}{.metadata.labels.cnpg\.io/reload}{"\n"}'
done
```

CNPG created the role and the database. Expect role `auth` (can log in) and database `auth` owned by `auth`:
```bash
kubectl -n mycloud-dev get databaseroles.postgresql.cnpg.io,databases.postgresql.cnpg.io
kubectl -n mycloud-dev exec db-1 -- psql -U postgres -c '\du auth' -c '\l auth'
```

The password in the Secret really works (end-to-end proof). Expect `auth | auth`.
The password goes in through stdin: anything in the `exec` command line ends up in the k3s audit log.
```bash
kubectl -n mycloud-dev get secret auth-db -o jsonpath='{.data.password}' | base64 -d \
| kubectl -n mycloud-dev exec -i db-1 -- sh -c 'read -r PGPASSWORD; export PGPASSWORD;
    psql -h 127.0.0.1 -U auth -d auth -c "select current_user, current_database();"'
```

### The auth service

Pods are running:
```bash
kubectl -n mycloud-dev get pods -l app.kubernetes.io/name=auth -o wide
```

Readiness and version through a port-forward:
```bash
kubectl -n mycloud-dev port-forward svc/auth 8082:80 &
curl -s localhost:8082/readyz; echo            # {"status":"ok","checks":{...:"ok"}}
curl -s localhost:8082/api/auth/version; echo  # {"service":"auth","version":"sha-..."}
kill %1
```

Through the Ingress (browser, Cloudflare Access login for dev/qa):
- `https://cloud-dev.mohedine.dev/api/auth/version` → auth's JSON
- `https://cloud-dev.mohedine.dev/version` → still the api's JSON

Prod (public, no Access):
```bash
curl -s https://cloud.mohedine.dev/api/auth/version; echo
```

### Promotion

GitHub → **Actions → promote → Run workflow** → `qa`, then `prod` (approve).
Then check `auth-qa` / `auth-prod` are **Healthy**:
```bash
kubectl -n argocd get app auth-qa auth-prod
```

---

## M2 PR 1: schema & migrations (local)

Run from `backend/`.

### Generate the sqlc code

Run after any change to `internal/auth/queries/*.sql` or `internal/auth/migrations/*.sql`:
```bash
docker run --rm -v "$PWD":/src -w /src sqlc/sqlc:1.31.1 generate
ls internal/auth/store        # db.go  invites.sql.go  models.go  sessions.sql.go  users.sql.go
grep -h '^type .*Params struct' -A4 internal/auth/store/*.go
```

Dependencies, format, static checks:
```bash
go mod tidy
gofmt -l .        # must print nothing
go vet ./...
```

### Tests against a real Postgres

Start a throwaway Postgres and run all tests. The migration tests must **run**, not skip:
```bash
docker run -d --name pg-test -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app \
  -e POSTGRES_DB=app -p 5432:5432 postgres:18
export TEST_DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable'
go test -race ./...
go test -v ./internal/auth/migrations/
```

### Run the service locally

Expect `{"status":"ok","checks":{"migrations":"ok","postgres":"ok"}}` and a log line
`"migration applied","version":1,"file":"00001_init.sql"`:
```bash
# since M3 auth needs TOTP_KEY too; any random 32-byte key is fine locally
export TOTP_KEY=$(openssl rand -base64 32)
DATABASE_URL="$TEST_DATABASE_URL" go run ./cmd/auth &
sleep 3; curl -s localhost:8080/readyz; echo
```

Tables were created. Expect `users`, `invites`, `sessions`, `goose_db_version`:
```bash
docker exec pg-test psql -U app -d app -c '\dt'
```

Clean up:
```bash
kill %1; docker rm -f pg-test
```

---

## M2 PR 1: in the cluster

Migrations ran at startup:
```bash
kubectl -n mycloud-dev logs deploy/auth | grep -i migration
```

Tables exist in database `auth` and are owned by `auth`:
```bash
kubectl -n mycloud-dev exec db-1 -- psql -U postgres -d auth -c '\dt'
```

Which migration versions are applied:
```bash
kubectl -n mycloud-dev exec db-1 -- psql -U postgres -d auth -c 'select version_id, is_applied from goose_db_version'
```

Readiness now includes migrations. Expect `"migrations":"ok","postgres":"ok"`:
```bash
kubectl -n mycloud-dev port-forward svc/auth 8082:80 &
curl -s localhost:8082/readyz; echo
kill %1
```

---

## M3 PR 1: TOTP encryption key

Unit tests (laptop):
```bash
cd backend && go test -race ./internal/auth/keyring ./cmd/auth
```

The chart renders an `ExternalSecret` and the `TOTP_KEY` env var. Expect `ExternalSecret/auth-totp`
with `key: mycloud/dev/auth-totp`, and `TOTP_KEY` reading Secret `auth-totp`:
```bash
helm template auth deploy/charts/service -n mycloud-dev \
  -f deploy/services/auth/values.yaml -f deploy/services/auth/dev/values.yaml \
  | yq 'select(.kind == "ExternalSecret") | .metadata.name, .spec.data[0].remoteRef.key'
helm template auth deploy/charts/service -n mycloud-dev \
  -f deploy/services/auth/values.yaml -f deploy/services/auth/dev/values.yaml \
  | yq 'select(.kind == "Deployment") | .spec.template.spec.containers[0].env[] | select(.name == "TOTP_KEY")'
```

Vault has one key per environment, 32 bytes each. Expect `32` three times and three different hashes:
```bash
for e in dev qa prod; do
  vault kv get -field=key secret/mycloud/$e/auth-totp | base64 -d | wc -c
  vault kv get -field=key secret/mycloud/$e/auth-totp | sha256sum
done
```

In the cluster: synced, and equal to Vault. Expect `SecretSynced True` and `OK` three times:
```bash
for e in dev qa prod; do
  kubectl -n mycloud-$e get externalsecret auth-totp
  a=$(vault kv get -field=key secret/mycloud/$e/auth-totp | sha256sum)
  b=$(kubectl -n mycloud-$e get secret auth-totp -o jsonpath='{.data.key}' | base64 -d | sha256sum)
  [ "$a" = "$b" ] && echo "$e OK" || echo "$e DIFFERENT"
done
```

auth started with the key. Expect `"totp_key_id":1` in the start-up log:
```bash
kubectl -n mycloud-dev logs deploy/auth | grep listening
```

---

## M3 PR 2: 2FA setup, enable, disable

Unit and database tests (laptop, `backend/`, with `TEST_DATABASE_URL` set). The 2FA tests must **run**, not skip:
```bash
go test -race ./internal/auth/totp ./internal/auth
go test -v -run 'TOTP|Recovery' ./internal/auth | grep -E '^(--- |ok)'
```

The migration ran in the cluster. Expect `user_totp` and `recovery_codes`:
```bash
kubectl -n mycloud-dev logs deploy/auth | grep 00002_totp
kubectl -n mycloud-dev exec db-1 -- psql -U postgres -d auth -c '\dt'
```

End to end against dev, through a port-forward. `oathtool` plays the authenticator app
(`sudo apt install oathtool`); you can also scan the URI with a real app instead.
```bash
kubectl -n mycloud-dev port-forward svc/auth 8082:80 &
B=http://localhost:8082; J='Content-Type: application/json'
read -rp 'email: ' EMAIL; read -rsp 'password: ' PW; echo
TOK=$(curl -si $B/api/auth/login -H "$J" -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" \
      | sed -n 's/^set-cookie: __Host-session=\([^;]*\).*/\1/ip')
C="Cookie: __Host-session=$TOK"

SECRET=$(curl -s -X POST $B/api/auth/2fa/setup -H "$J" -H "$C" -d '{}' | jq -r .secret)
curl -s $B/api/auth/2fa/enable -H "$J" -H "$C" -d "{\"code\":\"$(oathtool --totp -b $SECRET)\"}" | jq   # 10 recovery codes
curl -s $B/api/auth/me -H "$C" | jq .totpEnabled                                                    # true

sleep 31   # a code works only once: wait for the next one
curl -s -o /dev/null -w '%{http_code}\n' $B/api/auth/2fa/disable -H "$J" -H "$C" \
  -d "{\"password\":\"$PW\",\"code\":\"$(oathtool --totp -b $SECRET)\"}"                        # 204
unset PW TOK SECRET C; kill %1
```

The database only holds ciphertext. While 2FA is on, `secret_enc` starts with `\x0101` (format 1, key 1)
and is 50 bytes (2 + 12 nonce + 20 secret + 16 tag):
```bash
kubectl -n mycloud-dev exec db-1 -- psql -U postgres -d auth -c \
  "SELECT left(encode(secret_enc,'hex'),4) AS header, length(secret_enc), enabled_at IS NOT NULL AS enabled FROM user_totp"
```

Expected errors: wrong code `400 invalid_code`, same code twice `400 invalid_code`, wrong password on disable
`403 wrong_password`, setup while on `409 totp_already_enabled`, enable without setup `409 totp_not_set_up`.

---

## M3 PR 3: login with 2FA

Tests (laptop, `backend/`, `TEST_DATABASE_URL` set). They must **run**, not skip:
```bash
go test -race ./internal/auth
go test -v -run 'LoginWithTOTP|LoginTOTP|RecoveryCode|ResetTOTP|AdminNeeds2FA' ./internal/auth | grep -E '^(--- |ok)'
```

The migration ran in dev. Expect `00003_mfa_challenges.sql` and a table `mfa_challenges`:
```bash
kubectl -n mycloud-dev logs deploy/auth | grep 00003
kubectl -n mycloud-dev exec db-1 -- psql -U postgres -d auth -c '\d mfa_challenges'
```

Login with 2FA against dev, through a port-forward (`oathtool` plays the phone; `SECRET` is the one from
the PR 2 setup, or scan with a real app and type its code instead):
```bash
kubectl -n mycloud-dev port-forward svc/auth 8082:80 &
B=http://localhost:8082; J='Content-Type: application/json'
read -rp 'email: ' EMAIL; read -rsp 'password: ' PW; echo
# Step 1: 200 {"mfaRequired":true} and a __Host-mfa cookie, no session yet
MFA=$(curl -si $B/api/auth/login -H "$J" -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" \
      | sed -n 's/^set-cookie: __Host-mfa=\([^;]*\).*/\1/ip')
# Step 2: the code. Expect 204 and a __Host-session cookie
curl -si $B/api/auth/login/totp -H "$J" -H "Cookie: __Host-mfa=$MFA" \
  -d "{\"code\":\"$(oathtool --totp -b $SECRET)\"}" | grep -iE '^HTTP|set-cookie'
unset PW MFA; kill %1
```

The logs show which second factor was used (`totp` or `recovery_code`), never the code:
```bash
kubectl -n mycloud-dev logs deploy/auth | grep -E '2fa code required|second_factor|recovery code used|challenge used up'
```

Admin features need 2FA. An admin without it gets `403 totp_required` on `POST /api/auth/invites`, can still log in,
and turns it on with `/api/auth/2fa/setup` + `/enable`.

Lost phone **and** recovery codes: remove 2FA from the account (it also ends all its sessions).
Check who is asking first:
```bash
kubectl -n mycloud-dev exec deploy/auth -- /app reset-2fa --email someone@example.com
```

| Status | Error | Meaning |
|---|---|---|
| 200 | `{"mfaRequired":true}` | Password OK, send the code to `/api/auth/login/totp` |
| 400 | `invalid_code` | Wrong, expired or already used code (counts as an attempt) |
| 401 | `mfa_expired` | No `__Host-mfa` cookie, older than 5 minutes, or 5 wrong codes: log in again |
| 403 | `totp_required` | Admin route without 2FA on |

---

## Connect to the database by hand

**SQL shell inside the database pod** (superuser, no password; on prod use the primary,
see `kubectl get clusters.postgresql.cnpg.io -n mycloud-prod`):
```bash
kubectl -n mycloud-dev exec -it db-1 -- psql -U postgres
#  \l   list databases      \du  list roles      \c auth   switch to database auth
#  \dt  list tables         \q   quit
```

**From your PC with psql or a GUI**, as the `auth` user.

Terminal 1 (tunnel):
```bash
kubectl -n mycloud-dev port-forward svc/db-rw 15432:5432
```

Terminal 2:
```bash
export PGPASSWORD=$(kubectl -n mycloud-dev get secret auth-db -o jsonpath='{.data.password}' | base64 -d)
psql "host=localhost port=15432 user=auth dbname=auth sslmode=require"
unset PGPASSWORD
```
GUI (DBeaver, pgAdmin…): host `localhost`, port `15432`, db `auth`, user `auth`, SSL `require`.

As the `app` user (the api's database) instead:
```bash
export PGPASSWORD=$(kubectl -n mycloud-dev get secret db-app -o jsonpath='{.data.password}' | base64 -d)
psql "host=localhost port=15432 user=app dbname=app sslmode=require"
unset PGPASSWORD
```

Read-only on prod (replicas only, so you can't change data by accident):
```bash
kubectl -n mycloud-prod port-forward svc/db-ro 15433:5432
```

Use the password from Vault instead of the cluster (on your PC, logged in to Vault):
```bash
export PGPASSWORD=$(vault kv get -field=password secret/mycloud/dev/auth-db)
psql "host=localhost port=15432 user=auth dbname=auth sslmode=require"
unset PGPASSWORD
```

> Never expose Postgres through the Cloudflare Tunnel or an Ingress.
> Schema changes go through migrations, not hand-typed SQL.

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| auth pod `CreateContainerConfigError: secret "auth-db" not found` | ESO hasn't created it yet. `kubectl -n mycloud-dev get externalsecret auth-db`, and `kubectl -n argocd get app postgres-dev` |
| Postman folder 2 stops at "Login (admin): 2FA code" with `adminTotpSecret is empty` | The admin has 2FA but Postman doesn't know the secret. Use a fresh local database, or `reset-2fa` the local admin |
| Every 2FA code is `invalid_code` | Clock drift: codes are accepted ±30 s. Check the phone's time and `timedatectl` on the nodes |
| 2FA request returns 500, log says `open totp secret` | The row was sealed with another key: `auth-totp` in Vault was changed. Restore the old key, or reset that user's 2FA |
| auth exits at start: `TOTP_KEY is required` / `must be 32 bytes` | Secret `auth-totp` missing or wrong. Check `vault kv get secret/mycloud/<env>/auth-totp` and `kubectl -n mycloud-<env> get externalsecret auth-totp` |
| `auth-*` app stuck in sync on wave -1 | ESO can't read the Vault path (written before the merge?). `kubectl -n mycloud-<env> describe externalsecret auth-totp` |
| ExternalSecret `SecretSyncedError` | Vault path or key missing (`vault kv get secret/mycloud/dev/auth-db`) or the store is not `Valid` (`kubectl get clustersecretstore vault`) |
| Password changed in Vault, nothing happens | ESO re-reads every hour. Force it: `kubectl -n mycloud-dev annotate externalsecret auth-db force-sync=$(date +%s) --overwrite`, then restart auth |
| `password authentication failed for user "auth"` | CNPG hasn't applied the role yet: `kubectl -n mycloud-dev describe databaserole auth` |
| `database "auth" does not exist` | `kubectl -n mycloud-dev describe database auth` (the role must exist first) |
| Argo `auth-*`: `Unable to resolve '$values/...'` | The second source (`ref: values`) is missing in the ApplicationSet, or a path typo |
| CI `helm lint` / `helm template` fails with `… is required` | A value is missing in `deploy/services/<svc>/<env>/values.yaml` |
| `promote` fails with `auth has no valid tag in dev` | Dev was never built; push a backend change or re-run the `backend` workflow |
| `auth-qa` / `auth-prod` Degraded with `ImagePullBackOff` | Tag is still `sha-REPLACEME`; run the promote workflow |
| `sqlc generate` fails on a query | Check the error; usually a typo in `sqlc.yaml` or a query |
| `go vet` fails with `undefined: uuid` | Run `go mod tidy` after `sqlc generate` |
| CI `sqlc diff` fails | Re-run `sqlc generate` and commit `internal/auth/store` |
| `/readyz` stays `migrations: fail` | `kubectl -n mycloud-dev logs deploy/auth`; `permission denied` means `auth` doesn't own the database (check `\l auth`) |
| auth pod `OOMKilled` | The memory limit (256Mi) is missing from `deploy/services/auth/values.yaml` |