# overlays

One folder per environment (`dev`, `qa`, `prod`). Each has:

- `kustomization.yaml` – namespace, `env` label, image name + tag (`images:` transformer) and a JSON patch that sets the Ingress host.
- `secret-generator.yaml` – a **ksops** generator: decrypts the SOPS file at build time.
- `secrets/` – encrypted secrets (safe to commit, only decryptable with the age key).

The image tag line is rewritten by CI / `promote.yml` with `kustomize edit set image`.
