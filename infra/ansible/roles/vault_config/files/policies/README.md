# policies

Vault ACL policies. The task writes each file listed in `vault_config_policies` as a policy with the same name,
and rewrites it when the file changes.

| File | Who uses it | What it allows |
|------|-------------|----------------|
| `admin.hcl` | You (userpass login), replaces the root token | Manage audit devices, auth methods, secrets engines, policies, leases; full access to `secret/*`; take and restore Raft snapshots. |
| `eso.hcl` | External Secrets Operator in the cluster | **Read only** on `secret/data/mycloud/*` and `secret/data/platform/*`. |
| `snapshot.hcl` | The daily backup job on the VPS | Only `read` on `sys/storage/raft/snapshot` (save a snapshot, nothing else). |
