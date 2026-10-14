# argocd

- `install/` – installs Argo CD (with SOPS support so it can decrypt secrets).
- `project.yaml` – Argo project for our services (`mycloud-*` namespaces).
- `platform.yaml` – Argo project for shared platform components.
- `applicationset.yaml` – auto-creates one Argo app per service/environment from `deploy/apps`.
