# service (Helm chart)

One **generic chart** that deploys any MyCloud Go service (auth, later space, upload, file, ...).
Instead of copying Deployment/Service/Ingress YAML per service, a service is just a small values file.

## Files

- `Chart.yaml` – chart name and version.
- `values.yaml` – default values (resources, security context, probes port...). Every service inherits these.
- `templates/` – Kubernetes manifests written as Go templates:
  - `_helpers.tpl` – reusable snippets (`service.name`, `service.selectorLabels`, `service.labels`). Files starting with `_` are not rendered on their own.
  - `deployment.yaml` – pods, probes (`/healthz`, `/readyz`), resources, hardened security context.
  - `service.yaml` – ClusterIP Service in front of the pods.
  - `ingress.yaml` – Traefik Ingress, rendered only when `ingress.enabled: true`.
  - `externalsecret.yaml` – one `ExternalSecret` per `externalSecrets` entry: ESO copies a Vault path into a Secret (sync wave -1, so it exists before the pods start).

## Templating concepts used

- **Go templates**: `{{ .Values.x }}` reads a value, `{{- if ... }}` / `{{- range ... }}` add conditionals and loops.
- **`include` + `nindent`**: reuse the helper labels and indent them correctly inside the YAML.
- **`required`**: fails the render with a clear message if a mandatory value (image tag, environment) is missing.
- **`toYaml`**: copies a whole values block (`resources`, `securityContext`) into the manifest.
- **Release name = service name**: Argo CD sets `releaseName` to the service folder name, so `auth` becomes the Deployment, Service and label name.
- **Secrets from Vault**: `externalSecrets` lists Vault paths (rendered with `tpl`, so `{{ .Values.environment }}` works); `secretEnv` maps env vars to Secret keys. Values never appear in Git.
- **Conditional database wiring**: with `database.enabled: true` the Deployment reads user/password from a Secret and builds `DATABASE_URL` using Kubernetes `$(VAR)` expansion.

## Layered values (last wins)

1. `charts/service/values.yaml` – defaults
2. `deploy/services/<svc>/values.yaml` – per service
3. `deploy/services/<svc>/<env>/values.yaml` – per environment (host, image tag)

Try it locally:

```bash
helm template auth deploy/charts/service -n mycloud-dev \
  -f deploy/services/auth/values.yaml -f deploy/services/auth/dev/values.yaml
```
