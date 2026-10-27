# overlays

One folder per environment (`dev`, `qa`, `prod`). Each has:

- `kustomization.yaml` – namespace, `env` label, image name + tag (`images:` transformer) and a JSON patch that sets the Ingress host.

The api needs no secrets. If it ever does: write them to Vault and add an `ExternalSecret` here (see `postgres/overlays`).

The image tag line is rewritten by CI / `promote.yml` with `kustomize edit set image`.
