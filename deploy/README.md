# deploy

Everything Kubernetes, applied by Argo CD (nothing is applied by hand).

- `apps/` – Kustomize apps (base + per-env overlays): `api` (hello-world API) and `postgres` (CNPG cluster + roles).
- `charts/` – our generic Helm chart `service` for Go services.
- `services/` – values files per service/env that feed the chart (e.g. `auth`).
- `argocd/` – Argo CD install, projects and the ApplicationSets that create the apps.
- `platform/` – shared cluster components (Cloudflare tunnel).

Secrets live in `secrets/*.sops.yaml` files, encrypted with SOPS + age (rules in `/.sops.yaml`); only the `data`/`stringData` values are encrypted, so diffs stay readable.
