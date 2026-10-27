# snapshot: the daily backup job on the VPS. It can save snapshots and nothing else.
path "sys/storage/raft/snapshot" { capabilities = ["read"] }