# auth

Values for the auth service (`backend/cmd/auth`).

- `values.yaml` – image `mycloud-auth`, database `auth` (Secret `auth-db`), ingress path `/api/auth`,
  the internal port `8081` (ForwardAuth endpoint `/verify` for Traefik, never in the Ingress),
  `forwardAuth.provide` and `rateLimit.provide` (create the `forwardauth`, `strip-identity` and `ratelimit` Middlewares; auth's own Ingress uses `ratelimit` and `strip-identity`, so login works without a session),
  the Redis connection for failed-login counters (`REDIS_ADDR`, `REDIS_USER`, and `REDIS_PASSWORD` from Vault `mycloud/<env>/redis` via the ExternalSecret `auth-redis`),
  and the TOTP encryption key: `ExternalSecret` `auth-totp` from Vault `secret/mycloud/<env>/auth-totp` → env `TOTP_KEY`.
- `dev/`, `qa/`, `prod/` – environment name, image tag, public host (`cloud-dev.mohedine.dev`, ...) and `TOTP_ISSUER` (dev and qa; prod uses the default "MyCloud").
  `networkPolicy.enabled` is on in every environment: only Traefik may reach auth (see `deploy/apps/baseline/README.md`).
