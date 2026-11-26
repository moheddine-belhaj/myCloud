# baseline

Namespace-wide rules for each `mycloud-<env>` namespace. Argo CD creates one app per overlay (`baseline-dev`, …) through the ApplicationSet.

- `base/default-deny-ingress.yaml` – a NetworkPolicy that selects every pod and allows no ingress. A pod then only accepts traffic that its own policy allows:

| Pod | Allowed in | Policy |
|---|---|---|
| auth | Traefik → `http` (8080) and `internal` (8081) | Helm chart `networkPolicy` |
| api | Traefik → `http` (8080) | `apps/api/components/network-policy` |
| db (CNPG) | auth, api → 5432; other db instances and the CNPG operator → 5432, 8000 | `apps/postgres/components/network-policy` |

Not affected: kubelet probes (k3s lets node-to-pod traffic through), `k port-forward` and `k exec` (they enter the pod through the kubelet, not the pod network), and egress (only ingress is restricted).

A new service needs its own allow policy, otherwise nothing can reach it. For a Helm service set `networkPolicy.enabled: true`.

## Rollout

`overlays/dev` and `overlays/qa` exist. After the checks in `Troubleshooting/auth_service.md` (M4 PR 2) pass in qa, add `overlays/prod` (copy qa, change the namespace and `env`) together with the prod switches of the allow policies.
