# 11 · External Secrets Operator and Vault-backed secrets

Goal: the cluster reads every secret from Vault; Git holds only references.

## External Secrets Operator
1. Add ESO (Helm chart 2.14.0) as a `platform` Argo CD app in namespace `external-secrets`.
2. Bind ESO's ServiceAccount to `system:auth-delegator` so Vault can verify its token.
3. Add the `ClusterSecretStore vault` (Kubernetes auth, role `eso`, audience `https://kubernetes.default.svc.cluster.local`), limited to `mycloud-*` and `cloudflared`.
4. Verify with a test `ExternalSecret` and check Vault's and k3s's audit logs.

## Secrets from Vault
1. Write dev `auth-db` to Vault (`secret/mycloud/dev/auth-db`) and add its `ExternalSecret`.
2. Write the Cloudflare tunnel token to Vault (`secret/platform/cloudflared`) and add its `ExternalSecret`.
3. Rotate the tunnel token: Cloudflare → Vault → force-sync → rolling restart.
4. Write qa and prod `auth-db` to Vault and add their `ExternalSecret`s.

## Vault as the single source
1. Move every overlay to `ExternalSecret`s; Argo CD runs the upstream install without plugins.
2. Add a CI check that blocks any plain `kind: Secret` in `deploy/`.
3. Rotate the `auth-db` passwords for dev, qa and prod.
4. Retire the age key.
5. Record the decision in ADR `0003-secrets-in-vault-with-eso.md`.

Result: secret values live only in Vault, every read is audited, and Git holds `ExternalSecret` references.
