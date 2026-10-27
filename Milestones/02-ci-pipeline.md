# 02 · CI pipeline

Goal: every push is tested and produces a container image.

1. Write a multi-stage `Dockerfile` with a distroless runtime image and a `.dockerignore`.
2. Add unit tests for the API handlers.
3. Create `.github/workflows/backend.yml`:
    - **test**: `gofmt` check, `go vet`, `go test -race`.
    - **build**: Docker Buildx, push to GHCR as `ghcr.io/moheddine-belhaj/mycloud-api:sha-<commit>`, with a GHA layer cache.
4. Tag images by commit SHA so every image maps to exactly one commit.

Result: each push to `main` runs the tests and publishes an image to GitHub Container Registry.
