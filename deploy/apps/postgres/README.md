# postgres

PostgreSQL for each environment, managed by the **CloudNativePG (CNPG) operator** (installed from `argocd/platform.yaml`).
Each namespace (`mycloud-dev|qa|prod`) gets its own cluster named `db`.

- `base/`
  - `cluster.yaml` – the `Cluster` (1 instance, Postgres 18, 1 Gi on k3s `local-path`, small memory settings). Annotated so Argo CD never prunes/deletes it (deleting it would delete the data).
  - `databaserole-auth.yaml` – declares the login role `auth`; its password comes from the Secret `auth-db`.
  - `database-auth.yaml` – declares database `auth`, owned by role `auth`. Reclaim policy is *retain*: removing the YAML never drops data.
  - `kustomization.yaml` – lists the above.
- `components/network-policy/` – NetworkPolicy for the CNPG pods (`cnpg.io/cluster: db`): port 5432 only from auth and api; 5432 and 8000 (instance manager) from the other instances (replication in prod) and from the CNPG operator namespace `cnpg-system`. Included in every overlay.
- `overlays/<env>/` – sets the namespace and an **ExternalSecret** that reads `secret/mycloud/<env>/auth-db` from Vault into a `kubernetes.io/basic-auth` Secret `auth-db` (label `cnpg.io/reload`, so CNPG applies a changed password).

**One database + one role per service** follows the rule that no service touches another service's tables.
To add a service database, add a `DatabaseRole` + `Database` in `base/`, write the credentials to Vault (`secret/mycloud/<env>/<service>-db`) and add an `ExternalSecret` per overlay.
