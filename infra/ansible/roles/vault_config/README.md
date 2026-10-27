# roles/vault_config

Configures Vault itself (not the server) through its API. Runs on **this machine** with the `vault` CLI
and your current login (`vault login`). Every step reads the current state first and writes only when it
differs, so re-running changes nothing.

What it sets up: file audit device, KV v2 engine, policies, `userpass` + `kubernetes` auth, your admin
login, and the role External Secrets Operator uses to log in from the cluster.

| Folder | What it holds |
|--------|---------------|
| `defaults/` | Variables: admin user name, KV path, policy list, k3s API address, ESO identity. |
| `files/` | Static files the role reads (the policy HCL files). |
| `tasks/` | The configuration steps, in order. |
