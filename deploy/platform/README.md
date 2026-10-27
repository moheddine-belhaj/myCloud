# platform

Shared cluster components:

- `cloudflared/` – the Cloudflare Tunnel that exposes the app publicly (token from Vault).
- `secret-store/` – the `ClusterSecretStore` that lets External Secrets Operator read Vault.
