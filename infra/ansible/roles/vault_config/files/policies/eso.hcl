# eso: External Secrets Operator. Read-only, and only the paths the cluster needs.
path "secret/data/mycloud/*"  { capabilities = ["read"] }
path "secret/data/platform/*" { capabilities = ["read"] }