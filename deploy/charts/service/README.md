# service (Helm chart)

One **generic chart** that deploys any MyCloud Go service (auth, later space, upload, file, ...).
Instead of copying Deployment/Service/Ingress YAML per service, a service is just a small values file.

## Files

- `Chart.yaml` – chart name and version.
- `values.yaml` – default values (resources, security context, probes port...). Every service inherits these.
- `templates/` – Kubernetes manifests written as Go templates:
  - `_helpers.tpl` – reusable snippets (`service.name`, `service.selectorLabels`, `service.labels`). Files starting with `_` are not rendered on their own.
  - `deployment.yaml` – pods, probes (`/healthz`, `/readyz`), resources, hardened security context.
  - `service.yaml` – ClusterIP Service in front of the pods; with `internalPort` set, a second port `internal` that no Ingress uses (auth's ForwardAuth endpoint).
  - `ingress.yaml` – Traefik Ingress, rendered only when `ingress.enabled: true`. `ingress.middlewares` adds the `router.middlewares` annotation (`<namespace>-<name>@kubernetescrd`, built by the helper `service.middlewares`).
  - `middlewares.yaml` – only with `forwardAuth.provide: true` (auth): the Traefik Middlewares `forwardauth` and `strip-identity` for the namespace (see below).
  - `networkpolicy.yaml` – only with `networkPolicy.enabled: true`: the pods accept ingress only from the Traefik pods (`traefikNamespace` + `traefikLabels`), on ports `http` and `internal`.
  - `externalsecret.yaml` – one `ExternalSecret` per `externalSecrets` entry: ESO copies a Vault path into a Secret (sync wave -1, so it exists before the pods start).

## Templating concepts used

- **Go templates**: `{{ .Values.x }}` reads a value, `{{- if ... }}` / `{{- range ... }}` add conditionals and loops.
- **`include` + `nindent`**: reuse the helper labels and indent them correctly inside the YAML.
- **`required`**: fails the render with a clear message if a mandatory value (image tag, environment) is missing.
- **`toYaml`**: copies a whole values block (`resources`, `securityContext`) into the manifest.
- **Release name = service name**: Argo CD sets `releaseName` to the service folder name, so `auth` becomes the Deployment, Service and label name.
- **Secrets from Vault**: `externalSecrets` lists Vault paths (rendered with `tpl`, so `{{ .Values.environment }}` works); `secretEnv` maps env vars to Secret keys. Values never appear in Git.
- **Conditional database wiring**: with `database.enabled: true` the Deployment reads user/password from a Secret and builds `DATABASE_URL` using Kubernetes `$(VAR)` expansion.

## ForwardAuth (M4)

The auth release creates two Middlewares; any Ingress in the same namespace can list them in `ingress.middlewares`:

| Middleware | What it does |
|---|---|
| `strip-identity` | deletes client-sent `X-User-Id` / `X-User-Role` (a header set to `""` is removed by Traefik) |
| `forwardauth` | asks `http://auth.<ns>.svc.cluster.local:8081/verify` first; on `200` copies `X-User-Id` / `X-User-Role` onto the request, otherwise returns auth's `401`/`403` to the client |

Order matters: `strip-identity` first, then `forwardauth`. Public routes (auth's login, register) use only `strip-identity`. How a service reads the headers: `backend/internal/identity/README.md`.

## Layered values (last wins)

1. `charts/service/values.yaml` – defaults
2. `deploy/services/<svc>/values.yaml` – per service
3. `deploy/services/<svc>/<env>/values.yaml` – per environment (host, image tag)

Try it locally:

```bash
helm template auth deploy/charts/service -n mycloud-dev \
  -f deploy/services/auth/values.yaml -f deploy/services/auth/dev/values.yaml
```
