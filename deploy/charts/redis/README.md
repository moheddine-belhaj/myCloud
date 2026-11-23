# redis (Helm chart)

Redis for the **rate-limit counters** of auth (M4). One instance per environment (`redis-dev`, `redis-qa`, `redis-prod`), deployed by the ApplicationSet `mycloud-data` with the values in `deploy/data/redis/`.

## Files

- `values.yaml` – image, memory limit, Vault path, ACL user, NetworkPolicy, security context.
- `templates/configmap.yaml` – `redis.conf`.
- `templates/externalsecret.yaml` – reads the password from Vault and renders the ACL file `users.acl` into the Secret `redis-acl`.
- `templates/deployment.yaml` – one pod, mounts the config, the ACL Secret and an `emptyDir` for `/data`.
- `templates/service.yaml` – ClusterIP `redis:6379`.
- `templates/networkpolicy.yaml` – only the pods in `networkPolicy.allowFrom` (auth) may connect.

## Design

| Choice | Why |
|---|---|
| **No persistence** (`save ""`, `appendonly no`, no volume) | The data is counters that expire after minutes. Losing them on a restart only resets the limits, so there is nothing to back up and no PVC. |
| **1 replica, `Recreate`** | Two Redis pods would each hold half of the counters. Recreate stops the old pod before the new one starts. |
| **`maxmemory 64mb`, `volatile-ttl`** | Hard memory cap; every key has a TTL, so when full Redis drops the key that would expire soonest. |
| **Official `redis:8.2-alpine`** | Maintained upstream image. Bitnami stopped free image updates in 2025. |
| **`command: redis-server /etc/redis/redis.conf`** | Skips the image entrypoint (which would try to `chown` as root); the pod runs as uid 999 from the start. |
| **TCP probes** | `redis-cli ping` would need a password, because the default user is off. |
| **`protected-mode no`** | Protected mode only matters for a passwordless default user; here the default user is off and the NetworkPolicy limits who can connect. |
| **`bind * -::*`** | All IPv4 interfaces; the `-` makes IPv6 optional (the cluster has none, Redis logs a harmless warning). |

## Access control (ACL)

Vault `secret/mycloud/<env>/redis` holds `password` (64 hex characters). ESO renders:

```
user default off
user auth on #<sha256 of the password> resetkeys ~rl:* resetchannels -@all +@connection -@dangerous +get +incr +expire +ttl +pttl +del +exists +multi +exec +discard
```

- `default off` – anonymous connections can do nothing (`NOAUTH`).
- `#<sha256>` – Redis stores only the hash; the plain password goes to auth only (PR 4).
- `~rl:*` – the `auth` user may only touch keys starting with `rl:` (rate limit).
- `-@all … +incr +expire …` – only the commands the limiter needs. No `KEYS`, `FLUSHALL`, `CONFIG`, `EVAL`, `INFO`, `SHUTDOWN`; `-@dangerous` removes things like `CLIENT KILL` that `+@connection` would allow.

Tested against a real `redis-server`: the default user and wrong passwords are refused; `INCR`, `EXPIRE … NX`, `TTL`, `DEL`, `MULTI/EXEC` work on `rl:*`; `SET`, other key prefixes, `KEYS`, `FLUSHALL`, `CONFIG`, `CLIENT KILL`, `EVAL` are refused with `NOPERM`.

## Password rotation

Redis reads the ACL file only at start. After changing the password in Vault:

1. Force the ExternalSecret to sync (or wait up to 1 h).
2. `k -n mycloud-<env> rollout restart deploy/redis` (counters reset, which is fine).
3. Restart auth so it uses the new password (PR 4).

Commands and checks: `Troubleshooting/redis.md`.
