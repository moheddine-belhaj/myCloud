# data

Values for data services that have their **own** chart in `charts/<name>` (unlike Go services, which share `charts/service`).

```
data/<name>/
├── values.yaml        shared by all environments
├── dev/values.yaml    environment name (+ overrides)
├── qa/values.yaml
└── prod/values.yaml
```

The ApplicationSet `argocd/applicationset-data.yaml` discovers `deploy/data/*/*` and creates `<name>-<env>` in namespace `mycloud-<env>`, using the chart `deploy/charts/<name>`.

- `redis/` – rate-limit counters for auth (M4), chart `charts/redis`.

Postgres stays a Kustomize app (`apps/postgres`, CNPG).
