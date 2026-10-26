# ansible

Ansible setup that installs Vault on the VPS, configures Vault itself, and backs it up. Run all commands from this folder.

| File | What it does |
|------|--------------|
| `ansible.cfg` | Ansible settings: inventory and roles path, YAML output, no retry files, SSH pipelining. |
| `inventory.yml` | The hosts to manage: group `vault` with host `vps` (set `ansible_host` and `ansible_user`). No secrets. |
| `requirements.yml` | Collections to install (`community.general`, for the `ufw` module). |
| `vault.yml` | The playbook: runs the `vault` role on the `vault` group as root. |
| `vault-configure.yml` | Playbook: configures Vault through its API from this machine (`vault_config`), then sets up daily snapshots on the VPS (`vault_snapshot`). Needs `vault login` first. |
| `vault-backup-fetch.yml` | Playbook: copies the newest snapshot from the VPS to `~/backups/vault/` on this machine. |
| `group_vars/` | Variables shared by all playbooks (`vault_addr`). |
| `roles/` | The roles: `vault` (server), `vault_config` (Vault settings), `vault_snapshot` (backups). |

Order: `vault.yml` (install) → init/unseal by hand → `vault login` → `vault-configure.yml` → `vault-backup-fetch.yml` when you want a copy.

```bash
ansible-playbook vault.yml -K              # install the server (dry run: add --check --diff)
ansible-playbook vault-configure.yml -K    # configure Vault + daily snapshots
ansible-playbook vault-backup-fetch.yml -K # fetch the latest snapshot
```
