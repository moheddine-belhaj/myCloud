# tasks

| File | What it does |
|------|--------------|
| `main.yml` | The steps, in order: 1 preconditions (Tailscale up, certificate exists), 2 install Vault and hold the package, 3 create directories, 4 TLS certificate and certbot hook, 5 config, systemd drop-in, logrotate, `VAULT_ADDR`, 6 firewall (8200 on `tailscale0` only), 7 start the service and check its health. |
