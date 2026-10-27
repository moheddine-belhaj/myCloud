# tasks

| File | What it does |
|------|--------------|
| `main.yml` | 0 checks Vault is reachable, unsealed and you are logged in; 1 enables the file audit device (`/var/log/vault/audit.log`); 2 enables KV v2 at `secret/`; 3 writes the policies; 4 enables `userpass` and `kubernetes` auth; 5 creates your admin user (asks the password once, min. 16 characters, sent via stdin); 6 configures Kubernetes auth (k3s API address + CA from your kubeconfig) and the `eso` role (1 h tokens, policy `eso`). |
