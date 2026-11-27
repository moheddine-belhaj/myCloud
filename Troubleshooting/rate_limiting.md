# Rate limiting (M4 PR 4)

Three layers (design: `backend/internal/auth/README.md#rate-limiting`):

| Layer | Where | Limit | Answer |
|---|---|---|---|
| 1. Cloudflare | dashboard rule on `/api/auth/login*`, `/api/auth/register` | 10 requests / 10 s per IP | Cloudflare block page (`429`) |
| 2. Traefik | Middleware `ratelimit` on `/api/*` and `/api/auth` | 10 req/s, burst 20, per `CF-Connecting-IP` | `429 Too Many Requests` (plain text) |
| 3. auth + Redis | failed logins and wrong 2FA codes | 5 / 15 min per account+IP, 20 / 15 min per IP, 50 / 1 h per account | `429 {"error":"too_many_attempts"}` + `Retry-After` |

## 1. Check the deployment (per environment)

```bash
k -n mycloud-dev get middleware                        # forwardauth, ratelimit, strip-identity
k -n mycloud-dev get externalsecret auth-redis         # SecretSynced, True
k -n mycloud-dev logs deploy/auth | grep -i "rate limit" # nothing = OK; "rate limiting off" = REDIS_ADDR missing
k -n mycloud-dev get ingress -o custom-columns=NAME:.metadata.name,MW:.metadata.annotations.traefik\.ingress\.kubernetes\.io/router\.middlewares
```

## 2. Test layer 3: failed-login lock

Through auth directly; the `CF-Connecting-IP` header plays the client IP:

```bash
k -n mycloud-dev port-forward svc/auth 8080:80
for i in 1 2 3 4 5 6; do
  curl -s -o /dev/null -w "attempt $i: %{http_code}\n" localhost:8080/api/auth/login \
    -H 'Content-Type: application/json' -H 'CF-Connecting-IP: 203.0.113.7' \
    -d '{"email":"locktest@example.com","password":"wrong password!"}'
done
# attempt 1-5: 401, attempt 6: 429

curl -si localhost:8080/api/auth/login -H 'Content-Type: application/json' -H 'CF-Connecting-IP: 203.0.113.7' \
  -d '{"email":"locktest@example.com","password":"wrong password!"}' | grep -iE '^HTTP|retry-after|error'
# HTTP/1.1 429, Retry-After: ~900, "too_many_attempts"

k -n mycloud-dev logs deploy/auth | grep "login rate limited" | tail -1   # ip, rule, retry_after_s (no email)
```

Another `CF-Connecting-IP` is not locked (account+IP rule). Postman: folder "11. Rate limiting".

## 3. Read the counters

Keys use the SHA-256 of the lowercase email. The password goes through stdin:

```bash
H=$(printf '%s' 'locktest@example.com' | sha256sum | cut -d' ' -f1)
vault kv get -field=password secret/mycloud/dev/redis | \
  k -n mycloud-dev exec -i deploy/redis -- sh -c "
    read -r P; export REDISCLI_AUTH=\"\$P\"
    redis-cli --user auth GET rl:login:acctip:$H:203.0.113.7
    redis-cli --user auth TTL rl:login:acctip:$H:203.0.113.7
    redis-cli --user auth GET rl:login:ip:203.0.113.7
    redis-cli --user auth GET rl:login:acct:$H"
```

## 4. Unlock someone

The IP is in the `login rate limited` log line:

```bash
k -n mycloud-dev exec deploy/auth -- /app unlock --email locktest@example.com --ip 203.0.113.7
# unlocked locktest@example.com
```

Without `--ip` only the per-account counter is cleared; the account+IP lock then ends by itself within 15 minutes.

## 5. Test layer 2: Traefik

Through the real Traefik, 40 parallel requests from one "IP":

```bash
k -n kube-system port-forward svc/traefik 8000:80
seq 40 | xargs -P 20 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -H 'Host: cloud-dev.mohedine.dev' -H 'CF-Connecting-IP: 198.51.100.50' localhost:8000/version | sort | uniq -c
# about 20 × 200 (the burst), the rest 429
```

Always send `CF-Connecting-IP` in these tests: without it, every request lands in the same bucket.

## 6. Test fail open

Login must keep working when Redis is down:

```bash
k -n mycloud-dev scale deploy/redis --replicas=0
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"<your account>","password":"<your password>"}'      # 204 (or 200 with 2FA)
k -n mycloud-dev logs deploy/auth | grep "rate limit unavailable" | tail -1
```

Argo CD self-heal scales Redis back to 1 within seconds, so run the curl right after the scale command. The Go test `TestLoginFailsOpenWithoutRedis` covers the same case.

## 7. Cloudflare rule (layer 1)

Cloudflare dashboard → zone `mohedine.dev` → **Security → WAF → Rate limiting rules** → **Create rule**:

| Field | Value |
|---|---|
| Rule name | `mycloud login and register` |
| Expression (Edit expression) | `(http.host in {"cloud.mohedine.dev" "cloud-qa.mohedine.dev" "cloud-dev.mohedine.dev"} and (starts_with(http.request.uri.path, "/api/auth/login") or http.request.uri.path eq "/api/auth/register"))` |
| With the same characteristics | IP |
| When rate exceeds | 10 requests per 10 seconds |
| Then take action | Block, for 10 seconds |

The free plan offers a 10-second period and block time; take the closest values your plan shows. Test from your machine (each attempt also counts in auth, so use a made-up email):

```bash
for i in $(seq 15); do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST https://cloud.mohedine.dev/api/auth/login \
    -H 'Content-Type: application/json' -d '{"email":"cf-test@example.com","password":"wrong password!"}'
done
# first answers 401/429 from auth, then 429 from Cloudflare
```

Cloudflare → Security → Events shows the blocked requests.

## Common problems

| Symptom | Cause / fix |
|---|---|
| auth log `rate limiting off: REDIS_ADDR is empty` | auth runs without the M4 values: check `k -n mycloud-<env> get deploy auth -o yaml \| grep REDIS` |
| auth log `rate limit unavailable … WRONGPASS` | Redis still has an older ACL than Vault: `k -n mycloud-<env> rollout restart deploy/redis`, then `deploy/auth` |
| auth log `rate limit unavailable … NOPERM` | A command is missing in the Redis ACL (`aclUser.commands` in `deploy/charts/redis/values.yaml`); the limiter needs `get pttl incr expire del multi exec` |
| auth log `rate limit unavailable … i/o timeout` | Redis is down or the `redis` NetworkPolicy doesn't match auth: `k -n mycloud-<env> get pods -l app.kubernetes.io/name=redis`, `k -n mycloud-<env> get networkpolicy redis -o yaml` |
| Every user gets Traefik's `429` | Requests arrive without `CF-Connecting-IP` (all in one bucket): only happens when bypassing Cloudflare, e.g. in port-forward tests |
| `/api/*` gives `404`/`500` after the merge | Traefik can't find the `ratelimit` Middleware: `k -n mycloud-<env> get middleware ratelimit`, `k -n kube-system logs deploy/traefik \| grep -i middleware` |
| You locked yourself out | `unlock` (section 4), or wait for `Retry-After` |
| `ExternalSecret auth-redis` not synced | Vault entry missing: `vault kv get secret/mycloud/<env>/redis` (created in PR 3) |
