# templates

| File | Installed at | What it does |
|------|--------------|--------------|
| `vault-snapshot.sh.j2` | `/usr/local/sbin/vault-snapshot` | Renews the token, saves `vault-<UTC time>.snap` (`0600`), deletes snapshots older than the retention. |
| `vault-snapshot.service.j2` | `/etc/systemd/system/vault-snapshot.service` | One-shot unit that runs the script, after `vault.service`. |
| `vault-snapshot.timer.j2` | `/etc/systemd/system/vault-snapshot.timer` | Runs the service daily at 03:30 (random delay up to 30 min, catches up if the VPS was off). |
