# 06 · PostgreSQL with CloudNativePG

Goal: a managed PostgreSQL cluster per environment.

1. Install the CloudNativePG operator from its Helm chart (pinned version) as a `platform` Argo CD app.
2. Add `deploy/apps/postgres`: a `Cluster` named `db` (Postgres 18, k3s `local-path` storage, small memory settings).
3. Protect the cluster from pruning so Argo CD never deletes the data.
4. Add overlays for dev, qa and prod (prod with its own settings).
5. Add a pgx connection pool to the API (`internal/database`).
6. Make `/readyz` check the database connection.
7. Run tests in CI against a Postgres service container.

Result: every environment has its own Postgres cluster, and the API reports ready only when the database answers.
