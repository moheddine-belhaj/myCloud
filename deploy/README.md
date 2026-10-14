# deploy

Everything Kubernetes, applied by Argo CD.

- `apps/` – our own services (Kustomize base + per-environment overlays).
- `argocd/` – Argo CD install and the Applications that watch this repo.
- `platform/` – shared cluster components (e.g. the Cloudflare tunnel).
