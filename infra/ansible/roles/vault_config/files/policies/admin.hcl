# admin: day-to-day administration by a person. Replaces the root token.

# Audit devices
path "sys/audit"   { capabilities = ["read", "sudo"] }
path "sys/audit/*" { capabilities = ["create", "read", "update", "delete", "sudo"] }

# Auth methods (userpass, kubernetes) and their users and roles
path "sys/auth"    { capabilities = ["read"] }
path "sys/auth/*"  { capabilities = ["create", "read", "update", "delete", "sudo"] }
path "auth/*"      { capabilities = ["create", "read", "update", "delete", "list", "sudo"] }

# Secrets engines
path "sys/mounts"   { capabilities = ["read"] }
path "sys/mounts/*" { capabilities = ["create", "read", "update", "delete"] }

# Policies
path "sys/policies/acl"   { capabilities = ["list"] }
path "sys/policies/acl/*" { capabilities = ["create", "read", "update", "delete", "list"] }

# Application secrets (KV v2)
path "secret/*" { capabilities = ["create", "read", "update", "patch", "delete", "list"] }

# Raft snapshots: save, and restore into a test Vault
path "sys/storage/raft/snapshot"       { capabilities = ["read"] }
path "sys/storage/raft/snapshot-force" { capabilities = ["update", "sudo"] }

# Leases and token housekeeping
path "sys/leases/*" { capabilities = ["create", "read", "update", "delete", "list", "sudo"] }