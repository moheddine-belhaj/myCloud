# 0003. Secrets in self-hosted Vault, synced by External Secrets Operator

- Status: Accepted (supersedes the "SOPS + age" part of 0002, decision 7)
- Date: 2026-10-27
- Phase: 1 (Auth)

## Context

ADR 0002 chose SOPS + age: secrets encrypted in Git, decrypted by KSOPS inside the Argo CD repo-server.
That worked, but in practice it had gaps:

- **No audit trail.** Anyone with the age key (my laptop, the `sops-age` Secret in Argo CD) can decrypt
  every secret, and nothing records who read what.
- **Rotation is a commit.** Changing a password means re-encrypting a file, a PR and a sync. Old
  encrypted values stay in the history of a public repo forever, protected only by one key.
- **One key for everything.** dev, qa and prod share the same age key.
- **Argo CD runs custom code.** KSOPS needs `--enable-alpha-plugins --enable-exec` and an init container
  that installs binaries into the repo-server.

At the same time, auth (M2) adds more secrets (TOTP key next), so this is the moment to change.

## Decision

1. **HashiCorp Vault Community on the VPS**, reachable only over Tailscale (`vault.mohedine.dev:8200`,
   Let's Encrypt via DNS-01). Raft storage, 3 unseal key shares (threshold 2), root token revoked,
   file audit device, daily snapshots. Installed and configured with Ansible (`infra/ansible`).
2. **External Secrets Operator** (Helm chart via Argo CD) reads Vault with the Kubernetes auth method
   (role `eso`, read-only policy on `secret/mycloud/*` and `secret/platform/*`) and writes normal
   Kubernetes Secrets. One `ClusterSecretStore`, limited to the `mycloud-*` and `cloudflared` namespaces.
3. **Git holds `ExternalSecret` objects only** (which Vault path, which keys), never values. CI fails on
   any `kind: Secret` under `deploy/`.
4. **SOPS, age and KSOPS are removed**: no `.sops.yaml`, no encrypted files, no Argo CD patches, and the
   age key is destroyed.

## Alternatives considered

- **Keep SOPS + age:** no new server, but none of the gaps above get fixed.
- **Sealed Secrets:** removes the laptop key, but still no audit trail and still values in Git.
- **OpenBao:** the open-source Vault fork, same API. A good fallback if Vault's licence becomes a
  problem; Vault Community has more documentation and is what I'm likely to meet at work.
- **Cloud managers (Azure Key Vault, AWS Secrets Manager):** managed and audited, but tie the home lab to
  a cloud account and cost money; self-hosting is part of the learning goal.
- **Vault inside the cluster:** a secret store that runs on the cluster it protects can't help when the
  cluster is broken, and competes for the cluster's RAM.

## Consequences

- One more server to run: unseal after every VPS reboot (manual, 2 of 3 keys), upgrades (apt package on
  hold), backups (snapshots fetched with `vault-backup-fetch.yml`).
- If Vault is down or sealed, running pods keep their Secrets; only new secrets and changes wait. ESO
  retries.
- Every read is in Vault's audit log, and Kubernetes Secret reads are in the k3s API audit log.
- Rotation is `vault kv put` + a forced sync, without a commit.
- Old SOPS files stay in Git history. Every value they held was rotated or is unused, so destroying
  the age key loses nothing.
