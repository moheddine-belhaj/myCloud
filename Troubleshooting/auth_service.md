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
- [Connect to the database by hand](#connect-to-the-database-by-hand)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

| Tool | Used for |
|---|---|
| `kubectl` | everything in the cluster |
| `sops` + age key in `~/.config/sops/age/keys.txt` | reading encrypted secrets locally |
| `yq` | reading YAML |
| `helm` | rendering the chart (`sudo snap install helm --classic`) |
| `docker` | local Postgres for tests, running sqlc |
| `go` | building and testing the backend |

---

## M1: before committing

### Encrypted `auth-db` secrets (one per environment)

All three files exist and are encrypted. Expect `3` per file (username, password, mac):
```bash
ls deploy/apps/postgres/overlays/*/secrets/auth-db.sops.yaml
grep -c 'ENC\[' deploy/apps/postgres/overlays/*/secrets/auth-db.sops.yaml
```

Decrypted content is correct. Expect per environment: `auth-db`, `kubernetes.io/basic-auth`,
`true`, `auth`, `48` (the password length, so the password itself is never printed):
```bash
for e in dev qa prod; do
  echo "== $e"
  sops -d deploy/apps/postgres/overlays/$e/secrets/auth-db.sops.yaml \
  | yq '.metadata.name, .type, .metadata.labels."cnpg.io/reload", (.data.username | @base64d), (.data.password | @base64d | length)'
done
```

Each environment has a different password. Expect three different hashes:
```bash
for e in dev qa prod; do
  sops -d deploy/apps/postgres/overlays/$e/secrets/auth-db.sops.yaml | yq '.data.password' | sha256sum
done
```

Only values are encrypted; name, type and label stay readable (no `ENC[` here):
```bash
yq '.metadata, .type' deploy/apps/postgres/overlays/dev/secrets/auth-db.sops.yaml
```

`generators:` appears exactly once per overlay. Expect `1` each; `2` means the block was appended twice:
```bash
grep -c '^generators:' deploy/apps/postgres/overlays/*/kustomization.yaml
```

The generator points to the right file. Expect `- ./secrets/auth-db.sops.yaml` three times:
```bash
grep -A1 '^files:' deploy/apps/postgres/overlays/*/secret-generator.yaml
```

The postgres overlays render the same way CI does it (generators removed). Expect
`Cluster/db`, `DatabaseRole/auth`, `Database/auth`:
```bash
for e in dev qa prod; do
  cp deploy/apps/postgres/overlays/$e/kustomization.yaml /tmp/orig.yaml
  yq -i 'del(.generators)' deploy/apps/postgres/overlays/$e/kustomization.yaml
  echo "== $e"; kubectl kustomize deploy/apps/postgres/overlays/$e | yq '.kind + "/" + .metadata.name'
  cp /tmp/orig.yaml deploy/apps/postgres/overlays/$e/kustomization.yaml
done
```

git will commit the secret files (not ignored):
```bash
git check-ignore -v deploy/apps/postgres/overlays/*/secrets/auth-db.sops.yaml || echo "OK: not ignored"
git status --short deploy/apps/postgres
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

The `auth-db` Secret exists in every environment. Expect `kubernetes.io/basic-auth  reload=true` three times:
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

The password in the Secret really works (end-to-end proof). Expect `auth | auth`:
```bash
PW=$(kubectl -n mycloud-dev get secret auth-db -o jsonpath='{.data.password}' | base64 -d)
kubectl -n mycloud-dev exec db-1 -- env PGPASSWORD="$PW" \
  psql -h 127.0.0.1 -U auth -d auth -c 'select current_user, current_database();'
unset PW
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

Read a secret value from Git (on your PC, needs your age key):
```bash
sops -d deploy/apps/postgres/overlays/dev/secrets/auth-db.sops.yaml | yq '.data.password | @base64d'
```

> Never expose Postgres through the Cloudflare Tunnel or an Ingress.
> Schema changes go through migrations, not hand-typed SQL.

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `grep -c 'ENC\['` shows `0` or the file is empty | The sops pipe failed. Re-create that environment's secret |
| `generators:` count is `2` | Delete the duplicate block in the overlay's `kustomization.yaml` |
| `sops -d` fails with `no identity matched` | Your `keys.txt` doesn't match the public key in `.sops.yaml` |
| Argo: `failed to get the data key` | The `sops-age` secret in `argocd` doesn't match your key |
| auth pod `CreateContainerConfigError: secret "auth-db" not found` | `postgres-<env>` hasn't synced yet, or KSOPS failed. Check `kubectl -n argocd get app postgres-dev` |
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