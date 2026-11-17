# auth

Values for the auth service (`backend/cmd/auth`).

- `values.yaml` – image `mycloud-auth`, database `auth` (Secret `auth-db`), ingress path `/api/auth`,
  the internal port `8081` (ForwardAuth endpoint `/verify` for Traefik, never in the Ingress),
  `forwardAuth.provide` (creates the `forwardauth` and `strip-identity` Middlewares; auth's own Ingress uses only `strip-identity`, so login works without a session),
  and the TOTP encryption key: `ExternalSecret` `auth-totp` from Vault `secret/mycloud/<env>/auth-totp` → env `TOTP_KEY`.
- `dev/`, `qa/`, `prod/` – environment name, image tag, public host (`cloud-dev.mohedine.dev`, ...) and `TOTP_ISSUER` (dev and qa; prod uses the default "MyCloud").
  `networkPolicy.enabled` is on in dev only until the M4 PR 2 checks pass (see `deploy/apps/baseline/README.md`).
