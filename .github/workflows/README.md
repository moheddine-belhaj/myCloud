# workflows

GitHub Actions pipelines.

- `backend.yml` – on push/PR:
  - **test**: `gofmt` check, `go vet`, `go test -race` with a Postgres service container.
  - **validate-manifests**: renders Kustomize apps and Helm charts (`helm lint` + `helm template`), validates them with `kubeconform`, and fails if any rendered manifest is a `kind: Secret` (secrets come from Vault through `ExternalSecret`s).
  - **build**: *matrix* over `api` and `auth`, builds and pushes `ghcr.io/<owner>/mycloud-<service>:sha-<commit>` with Buildx and a GHA layer cache.
  - **deploy-dev**: bumps the image tag in the dev overlay/values with `yq`/`kustomize edit`, commits `[skip ci]` via a deploy key; Argo CD then syncs.
- `promote.yml` – manual (`workflow_dispatch`) action: copies every service's tag dev → qa or qa → prod. Uses GitHub *environments* so prod can require approval. Build once, promote the same image.
