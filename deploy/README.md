# deploy

Everything Kubernetes, applied by Argo CD (nothing is applied by hand).

- `apps/` – Kustomize apps (base + per-env overlays): `api` (hello-world API) and `postgres` (CNPG cluster + roles).
- `charts/` – our generic Helm chart `service` for Go services.
- `services/` – values files per service/env that feed the chart (e.g. `auth`).
- `argocd/` – Argo CD install, projects and the ApplicationSets that create the apps.
- `platform/` – shared cluster components (Cloudflare tunnel, External Secrets store).

No secret values in Git. Secrets live in Vault (on the VPS); each one is described here by an `ExternalSecret`
(Vault path + keys), and External Secrets Operator creates the Kubernetes Secret. CI fails on any `kind: Secret`.
