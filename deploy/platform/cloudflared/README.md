# cloudflared

Runs the Cloudflare Tunnel inside the cluster.

- `namespace.yaml` – the namespace it lives in.
- `deployment.yaml` – the cloudflared pods.
- `secret-generator.yaml` – decrypts the tunnel credentials.
- `secrets/` – the encrypted tunnel token (SOPS).
- `kustomization.yaml` – ties it together.
