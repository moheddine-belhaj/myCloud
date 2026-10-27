# roles/vault

Ansible role that installs Vault from HashiCorp's apt repo and configures it to listen only on Tailscale.

| Folder | What it holds |
|--------|---------------|
| `defaults/` | Variables (domain, ports, directories, retention) with their default values. |
| `tasks/` | The installation steps, in order. |
| `handlers/` | Reload/restart actions triggered when config changes. |
| `templates/` | Config files rendered onto the VPS. |
