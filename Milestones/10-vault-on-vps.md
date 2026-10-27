# 10 · Vault on the VPS

Goal: a self-hosted secrets manager with an audit trail, outside the cluster.

## Network, DNS and TLS
1. Join the VPS to the tailnet; tag the nodes `tag:k3s` and the VPS `tag:vault`.
2. Write the Tailscale ACL: members → all; `tag:k3s` → `tag:vault:8200`; `tag:vault` → `tag:k3s:6443`; with `tests` (`infra/tailscale/policy.hujson`).
3. Add `vault.mohedine.dev` in Cloudflare as an A record to the VPS's Tailscale IP, DNS only.
4. Get a Let's Encrypt certificate with certbot DNS-01 and a scoped Cloudflare API token.
5. Test that pods on both nodes reach the VPS on port 8200, and that the public IP doesn't.

## Vault server (Ansible role `vault`)
1. Set up `infra/ansible` (`ansible.cfg`, `inventory.yml`, `requirements.yml`, playbook `vault.yml`).
2. Check preconditions: Tailscale IP and certificate exist.
3. Install Vault from HashiCorp's signed apt repo and hold the package.
4. Create data, TLS and audit directories with tight permissions.
5. Install a certbot deploy hook that copies the certificate and reloads Vault.
6. Render `vault.hcl` (Raft storage, listener on the Tailscale IP only), a systemd drop-in (start after Tailscale), logrotate and `VAULT_ADDR`.
7. Allow port 8200 in `ufw` on `tailscale0` only; start Vault and check its health.
8. Initialise by hand (3 key shares, threshold 2) and unseal.

## Vault configuration (Ansible roles `vault_config`, `vault_snapshot`)
1. Enable the file audit device and KV v2 at `secret/`.
2. Write policies `admin`, `eso` (read-only on `secret/mycloud/*` and `secret/platform/*`) and `snapshot`.
3. Enable `userpass` and create the personal admin login; revoke the root token.
4. Enable Kubernetes auth (cluster CA, API over Tailscale, client-JWT mode) and role `eso` (1 h tokens).
5. Create an orphan periodic snapshot token and a daily snapshot timer (03:30, 14 days kept).
6. Add `vault-backup-fetch.yml` to copy the newest snapshot to the laptop.
7. Test a restore in Docker with the real unseal keys.

Result: Vault runs on the VPS, reachable only over Tailscale, audited, configured as code and backed up daily.
