# 0003. Dev, QA and prod as namespaces in one cluster with image promotion

- Status: Accepted
- Date: 2026-10-07
- Phase: 1 (GitOps)

## Context

The cluster now has two nodes (`cp1` control plane + `worker1`). We want separate dev, qa and prod
environments so changes can be tried before they reach real users, without buying more hardware.
CI builds images to GHCR (private) and Argo CD deploys from Git.

## Decision

- **One cluster, three namespaces:** `mycloud-dev`, `mycloud-qa`, `mycloud-prod`, labelled
  `mycloud.dev/env` and Pod Security `restricted`.
- **One branch, one folder per environment:** `deploy/apps/<app>/overlays/{dev,qa,prod}` on `main`.
  Kustomize overlays set namespace, image tag and per-env patches (prod: 2 replicas spread over nodes).
- **Argo CD ApplicationSet per app** generates `<app>-dev`, `<app>-qa`, `<app>-prod`, all auto-synced.
- **Build once, promote the same image:** push to `main` → test → build `sha-xxxxxxx` → CI writes the
  tag into the dev overlay. `promote.yml` (manual) copies dev → qa and qa → prod. The GitHub `prod`
  environment requires approval.

## Alternatives considered

- **Branch per environment (dev/qa/main):** branches drift, merges carry unrelated changes, and what
  runs where is hard to see. Rejected.
- **Separate cluster per environment:** strongest isolation, but needs hardware we don't have.
- **Rebuild per environment:** the image in prod would not be the one that was tested.
- **Argo CD Image Updater:** automates tag bumps, but adds a component; CI commits are simpler and
  visible in Git history.

## Consequences

- Environments share nodes: a runaway dev pod can starve prod. Requests/limits are mandatory, and
  ResourceQuotas per namespace + NetworkPolicies between namespaces come in phase 7.
- Each environment will need its own database, bucket, secrets and hostname (e.g.
  `dev.cloud.mohedine.dev`, `qa.cloud.mohedine.dev`, `cloud.mohedine.dev`), which roughly triples
  stateful resources. Dev/qa can run smaller instances.
- Every node needs GHCR pull credentials (`/etc/rancher/k3s/registries.yaml`), since pods can land on
  either node.
- The promote workflow and the CI deploy job push to `main`; branch protection must allow the bot.
