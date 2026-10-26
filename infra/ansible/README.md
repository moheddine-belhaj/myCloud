# ansible

Ansible setup that installs and configures Vault on the VPS. Run all commands from this folder.

| File | What it does |
|------|--------------|
| `ansible.cfg` | Ansible settings: inventory and roles path, YAML output, no retry files, SSH pipelining. |
| `inventory.yml` | The hosts to manage: group `vault` with host `vps` (set `ansible_host` and `ansible_user`). No secrets. |
| `requirements.yml` | Collections to install (`community.general`, for the `ufw` module). |
| `vault.yml` | The playbook: runs the `vault` role on the `vault` group as root. |
| `roles/` | The roles used by the playbook (only `vault` for now). |

Apply: `ansible-playbook vault.yml -K` (dry run: add `--check --diff`).
