# templates

Jinja2 files rendered onto the VPS. Don't edit the installed copies by hand.

| File | Installed at | What it does |
|------|--------------|--------------|
| `vault.hcl.j2` | `/etc/vault.d/vault.hcl` | Vault server config: Raft storage, TLS listener on the Tailscale IP, UI on. |
| `vault-override.conf.j2` | `/etc/systemd/system/vault.service.d/override.conf` | Starts Vault after Tailscale and keeps retrying until its IP exists. |
| `certbot-deploy-hook.sh.j2` | `/etc/letsencrypt/renewal-hooks/deploy/vault.sh` | After each certificate renewal, copies the cert to Vault's TLS dir and reloads Vault. |
| `logrotate.j2` | `/etc/logrotate.d/vault` | Rotates audit logs daily, keeps 30 days, makes Vault reopen the file. |
| `vault-env.sh.j2` | `/etc/profile.d/vault.sh` | Sets `VAULT_ADDR` for shells on the VPS. |
