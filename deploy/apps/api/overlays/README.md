# overlays

One folder per environment (`dev`, `qa`, `prod`). Each has:

- `kustomization.yaml` – namespace and image tag for that environment.
- `secret-generator.yaml` – tells Kustomize to decrypt the SOPS secret.
- `secrets/` – encrypted secrets (safe to commit, only decryptable with the age key).
