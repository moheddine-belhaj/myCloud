# cloudflared

Runs the Cloudflare Tunnel inside the cluster. One tunnel serves dev, qa and prod.

- `namespace.yaml` – the namespace it lives in.
- `deployment.yaml` – the cloudflared pods (2 replicas, one per node).
- `external-secret.yaml` – the tunnel token, synced from Vault (`secret/platform/cloudflared`, key `token`)
  into the Secret `cloudflared-tunnel` by External Secrets Operator.
- `kustomization.yaml` – ties it together.

Rotate the token: Cloudflare dashboard → tunnel → Rotate token → `vault kv put secret/platform/cloudflared token=-`
→ force-sync the ExternalSecret → `kubectl -n cloudflared rollout restart deploy/cloudflared`.
See `infra/README.md`.