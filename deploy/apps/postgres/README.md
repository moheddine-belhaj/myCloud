# postgres

PostgreSQL for each environment, managed by the **CloudNativePG (CNPG) operator** (installed from `argocd/platform.yaml`).
Each namespace (`mycloud-dev|qa|prod`) gets its own cluster named `db`.

- `base/`
  - `cluster.yaml` – the `Cluster` (1 instance, Postgres 18, 1 Gi on k3s `local-path`, small memory settings). Annotated so Argo CD never prunes/deletes it (deleting it would delete the data).
  - `databaserole-auth.yaml` – declares the login role `auth`; its password comes from the Secret `auth-db`.
  - `database-auth.yaml` – declares database `auth`, owned by role `auth`. Reclaim policy is *retain*: removing the YAML never drops data.
  - `kustomization.yaml` – lists the above.
- `overlays/<env>/` – sets the namespace and a **ksops generator** that decrypts `secrets/auth-db.sops.yaml` (SOPS + age) into a `kubernetes.io/basic-auth` Secret.

**One database + one role per service** follows the rule that no service touches another service's tables.
To add a service database, add a `DatabaseRole` + `Database` in `base/` and a secret per overlay.
