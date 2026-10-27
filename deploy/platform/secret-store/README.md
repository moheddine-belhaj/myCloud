# secret-store

How the cluster reads secrets from Vault (on the VPS, Tailscale only).

- `cluster-secret-store.yaml` – ClusterSecretStore `vault`: KV v2 at `secret/`, Kubernetes auth
  (role `eso`). Usable only from `mycloud-dev|qa|prod` and `cloudflared`.
- `auth-delegator.yaml` – lets ESO's ServiceAccount call TokenReview, which Vault uses to verify
  ESO's token (no reviewer token stored in Vault).

Vault paths: `secret/mycloud/<env>/<name>` and `secret/platform/<name>`.
Vault side (policy `eso`, role `eso`): `infra/ansible/roles/vault_config`.