# group_vars

Variables shared by every playbook and role.

| File | What it does |
|------|--------------|
| `all.yml` | `vault_addr`: the URL of the Vault server (used by `vault_config`, `vault_snapshot` and the snapshot script). Set it to your Vault's real address. |
