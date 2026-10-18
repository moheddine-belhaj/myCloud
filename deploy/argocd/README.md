# argocd

GitOps entry point: Argo CD watches this repo (`main`) and makes the cluster match it.

- `install/` – installs Argo CD with SOPS support (ksops) so it can decrypt secrets.
- `project.yaml` – AppProject `mycloud`: may only deploy to `mycloud-*` namespaces.
- `platform.yaml` – AppProject `platform` plus the platform apps: `cloudflared` and the `cnpg-operator` (Helm chart from the CNPG repo, pinned version).
- `applicationset.yaml` – **ApplicationSet** for Kustomize apps: one Argo app per folder `deploy/apps/<app>/overlays/<env>` (e.g. `api-dev`, `postgres-qa`).
- `applicationset-services.yaml` – ApplicationSet for Helm services: one app per `deploy/services/<svc>/<env>`, using the shared chart `charts/service` with two layered values files (*multi-source* app, the second source is only a `$values` reference).

## Concepts

- **ApplicationSet + Git directory generator**: adding a folder creates a new app, no extra Argo YAML.
- **goTemplate** in the sets: `{{ .path.basename }}` (= env) builds app name and namespace `mycloud-<env>`.
- **Automated sync with `prune` and `selfHeal`**: deleting a file deletes the resource; manual `kubectl` edits get reverted.
- **Two deployment styles side by side**: Kustomize (`apps/`) and Helm (`services/`).
