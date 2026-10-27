# 04 · Secrets with SOPS + age

Goal: keep per-environment secrets in Git, encrypted, and decrypt them inside Argo CD.

1. Generate an age key pair on the laptop (`~/.config/sops/age/keys.txt`).
2. Add `.sops.yaml`: encrypt only `data`/`stringData` in `deploy/**/secrets/*.sops.yaml`, so diffs stay readable.
3. Store the age private key in the cluster as Secret `sops-age` in namespace `argocd`.
4. Patch the Argo CD repo-server: an init container installs KSOPS and kustomize, and the age key is mounted.
5. Enable Kustomize exec plugins in `argocd-cm` (`--enable-alpha-plugins --enable-exec`).
6. Add a KSOPS generator per overlay (`secret-generator.yaml`) pointing to the encrypted files.
7. Add a CI check that fails if any file under `secrets/` isn't SOPS-encrypted.

Result: encrypted secrets live in Git and Argo CD decrypts them at build time.
