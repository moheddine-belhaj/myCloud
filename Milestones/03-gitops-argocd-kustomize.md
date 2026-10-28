# 03 · GitOps with Argo CD and Kustomize

Goal: Git is the single source of truth; three environments run from one cluster.

1. Install Argo CD with Kustomize from the pinned upstream manifest (`deploy/argocd/install/`).
2. Create the AppProject `mycloud`, allowed to deploy only to `mycloud-*` namespaces.
3. Structure the api with Kustomize: `deploy/apps/api/base` (Deployment, Service) and `overlays/dev|qa|prod`.
4. Set per environment: namespace, `env` label, image tag (`images:` transformer); prod runs 2 replicas.
5. Add an ApplicationSet with a Git directory generator: one Argo CD app per `deploy/apps/<app>/overlays/<env>`.
6. Enable automated sync with `prune` and `selfHeal`.
7. Extend CI with **deploy-dev**: bump the dev image tag with `kustomize edit set image` and commit `[skip ci]` through a deploy key.
8. Add `.github/workflows/promote.yml`: copy the tag dev → qa and qa → prod, with GitHub environments (prod needs approval).
9. Add **validate-manifests** to CI: render every overlay and validate it with `kubeconform`.

Result: dev deploys on every push; qa and prod get the same image through promotion.
