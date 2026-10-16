# workflows

GitHub Actions pipelines.

- `backend.yml` – lint, test, build and scan the backend image on every push/PR; pushes the image to GHCR and updates the dev tag.
- `promote.yml` – manual action that copies an image tag from dev → qa or qa → prod.
