# roles/vault_snapshot

Daily, encrypted Raft snapshots of Vault, taken **on the VPS** with a token that can only take snapshots.
Run by `vault-configure.yml` after `vault_config` (the token is created with your login on this machine).

| Folder | What it holds |
|--------|---------------|
| `defaults/` | Variables: snapshot directory, retention days, token file, token period. |
| `handlers/` | Reload systemd after the unit files change. |
| `tasks/` | The installation steps. |
| `templates/` | The snapshot script and its systemd service and timer. |
