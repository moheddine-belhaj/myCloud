# install

Kustomize setup that installs Argo CD (upstream `install.yaml`, pinned version) with no patches.

Secrets don't go through Argo CD: they live in Vault and External Secrets Operator writes them into the cluster
(`deploy/platform/secret-store/`). Git only holds `ExternalSecret` objects, never secret values.

Applied by hand (Argo CD doesn't manage itself):
```bash
kubectl apply -k deploy/argocd/install --server-side --force-conflicts
```
