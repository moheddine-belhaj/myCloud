# Redis (M4 PR 3)

Redis holds the rate-limit counters of auth. Chart: `deploy/charts/redis` (design in its README), values: `deploy/data/redis/`, apps: `redis-dev`, `redis-qa`, `redis-prod`.

## 1. Create the passwords in Vault (once)

64 hex characters per environment, piped straight into Vault, never printed:

```bash
export VAULT_ADDR=https://vault.mohedine.dev:8200
vault login
for e in dev qa prod; do
  openssl rand -hex 32 | tr -d '\n' | vault kv put secret/mycloud/$e/redis password=-
done
vault kv get -format=json secret/mycloud/dev/redis | jq '.data.data | keys'   # ["password"]
```

## 2. Create the Argo CD apps (once, after the merge)

```bash
k apply -f deploy/argocd/applicationset-data.yaml
k -n argocd get applications | grep redis          # redis-dev, redis-qa, redis-prod: Synced, Healthy
```

## 3. Check the deployment

```bash
k -n mycloud-dev get externalsecret redis-acl      # SecretSynced, True
k -n mycloud-dev get pods -l app.kubernetes.io/name=redis
k -n mycloud-dev logs deploy/redis | tail -5       # "Ready to accept connections tcp"
k -n mycloud-dev get networkpolicy redis

# ACL file as ESO rendered it, hash masked
k -n mycloud-dev get secret redis-acl -o jsonpath='{.data.users\.acl}' | base64 -d \
  | sed 's/#[0-9a-f]\{64\}/#<sha256>/'
```

## 4. Test access control

The password goes through stdin into an environment variable, never into a command line (the k3s audit log records `exec` commands).

```bash
# default user: refused
k -n mycloud-dev exec deploy/redis -- redis-cli PING
# NOAUTH Authentication required.

# user auth: allowed commands work, the rest is refused
vault kv get -field=password secret/mycloud/dev/redis | \
  k -n mycloud-dev exec -i deploy/redis -- sh -c '
    read -r P; export REDISCLI_AUTH="$P"
    redis-cli --user auth PING             # PONG
    redis-cli --user auth INCR rl:check    # 1
    redis-cli --user auth TTL rl:check     # -1
    redis-cli --user auth DEL rl:check     # 1
    redis-cli --user auth SET other 1      # NOPERM
    redis-cli --user auth KEYS "*"         # NOPERM
    redis-cli --user auth FLUSHALL         # NOPERM'
```

## 5. Test the NetworkPolicy

Only auth pods may connect. From any other pod the connection must time out:

```bash
k run np-redis --rm -it --restart=Never -n default --image=redis:8.2-alpine -- \
  sh -c 'timeout 5 redis-cli -h redis.mycloud-dev.svc.cluster.local PING; echo "exit=$?"'
# exit=143 (timed out) or "Connection refused" → blocked, as intended
# PONG would mean the policy is missing: k -n mycloud-dev get networkpolicy redis
```

The allowed path (auth → Redis) is proven in PR 4, when auth connects at start-up.

## 6. Rotate the password

```bash
openssl rand -hex 32 | tr -d '\n' | vault kv put secret/mycloud/dev/redis password=-
k -n mycloud-dev annotate externalsecret redis-acl force-sync=$(date +%s) --overwrite
k -n mycloud-dev rollout restart deploy/redis      # Redis reads the ACL file only at start
k -n mycloud-dev rollout restart deploy/auth       # from PR 4 on: auth picks up the new password
```

## Common problems

| Symptom | Cause / fix |
|---|---|
| ExternalSecret `SecretSyncedError` | Vault path or key missing: `vault kv get secret/mycloud/<env>/redis`; details in `k -n mycloud-<env> describe externalsecret redis-acl` |
| Pod stuck in `ContainerCreating`, event `secret "redis-acl" not found` | ESO hasn't synced yet (see above) |
| Pod `CrashLoopBackOff`, log `Error in user declaration` or `aclfile` | The rendered ACL file is wrong: check it with the command in step 3 |
| Log `Permission denied` on `/etc/redis/acl/users.acl` | The Secret volume must be readable by group 1000: `fsGroup: 1000`, `defaultMode: 0440` |
| Pod rejected: `violates PodSecurity "restricted"` | A security setting was removed from `values.yaml` (`runAsNonRoot`, `seccompProfile`, `capabilities.drop`) |
| `WRONGPASS` with the right password | Redis still has the old ACL: restart it after a rotation (step 6) |
| `NOPERM this user has no permissions to run the 'x' command` | Command not in `aclUser.commands`: add it only if the limiter really needs it |
| `ErrImagePull` / `toomanyrequests` from Docker Hub | Anonymous pull limit: wait, or pin the image to a digest that is already cached on the nodes |
| Log `Could not create server TCP listening socket ::*:6379` | Harmless: no IPv6 in the cluster (`bind * -::*` makes IPv6 optional) |
| Log `WARNING Memory overcommit must be enabled` | Harmless here: it matters only for saving to disk, which is off |
