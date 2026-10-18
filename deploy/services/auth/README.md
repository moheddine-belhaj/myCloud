# auth

Values for the auth service (`backend/cmd/auth`).

- `values.yaml` – image `mycloud-auth`, database `auth` (Secret `auth-db`), ingress path `/api/auth`.
- `dev/`, `qa/`, `prod/` – environment name, image tag and public host (`cloud-dev.mohedine.dev`, ...).
