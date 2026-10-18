# apps

Kustomize apps, one folder each, with `base/` (shared) and `overlays/<env>/` (dev, qa, prod).

- `api/` – the first hello-world API (Deployment, Service, Ingress).
- `postgres/` – CloudNativePG cluster and per-service database/role.

**Kustomize vs Helm here:** Kustomize patches plain YAML per environment (good for one-off infra). Go services use the Helm chart in `../charts/service` instead.
