# 07 · M1: Helm foundation and auth service

Goal: a reusable way to deploy Go services, and the auth service with its own database.

1. Extract the HTTP server into `internal/httpserver` with graceful shutdown.
2. Create the auth service (`cmd/auth`) with `/healthz`, `/readyz` and `/api/auth/version`.
3. Declare a role and a database per service in CNPG: `DatabaseRole auth` and `Database auth` (retain policy).
4. Add the `auth-db` credentials per environment as encrypted secrets (`kubernetes.io/basic-auth`, label `cnpg.io/reload`).
5. Write the generic Helm chart `deploy/charts/service` (Deployment, Service, Ingress, `DATABASE_URL` from the secret).
6. Add values per service and environment: `deploy/services/auth/values.yaml` plus `dev|qa|prod/values.yaml`.
7. Add a second ApplicationSet for Helm services (multi-source app with a `$values` reference).
8. Extend CI: build a matrix of services (`api`, `auth`), and `helm lint` + `helm template` + `kubeconform` per environment.
9. Extend `promote.yml` to promote every service's tag together.

Result: `auth-dev`, `auth-qa` and `auth-prod` run from one chart, each with its own database.
