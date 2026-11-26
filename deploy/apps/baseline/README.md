# baseline

Namespace-wide rules for each `mycloud-<env>` namespace. Argo CD creates one app per overlay (`baseline-dev`, …) through the ApplicationSet.

- `base/default-deny-ingress.yaml` – a NetworkPolicy that selects every pod and allows no ingress. A pod then only accepts traffic that its own policy allows:

| Pod | Allowed in | Policy |
|---|---|---|
| auth | Traefik → `http` (8080) and `internal` (8081) | Helm chart `networkPolicy` |
| api | Traefik → `http` (8080) | `apps/api/components/network-policy` |
| db (CNPG) | auth, api → 5432; other db instances and the CNPG operator → 5432, 8000 | `apps/postgres/components/network-policy` |
| redis | auth → 6379 | Helm chart `charts/redis` (`networkPolicy`) |

Not affected: kubelet probes (k3s lets node-to-pod traffic through), `k port-forward` and `k exec` (they enter the pod through the kubelet, not the pod network), and egress (only ingress is restricted).

A new service needs its own allow policy, otherwise nothing can reach it. For a Helm service set `networkPolicy.enabled: true`.

## Rollout

Active in dev, qa and prod. Rolled out one environment at a time (dev → qa → prod), each checked with `Troubleshooting/auth_service.md` (M4 PR 2) before the next. A new environment needs its own overlay here plus the allow policies: `networkPolicy.enabled` in the auth values and the `network-policy` component in the api and postgres overlays.
