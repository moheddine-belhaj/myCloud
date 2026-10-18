# services

Configuration of each Go service, deployed with the shared chart [`charts/service`](../charts/service).
No YAML manifests here, only **values files**.

```
services/<service>/
├── values.yaml        shared by all environments (image repository, ingress path, database name)
├── dev/values.yaml    environment name, image tag, public host
├── qa/values.yaml
└── prod/values.yaml
```

- The `image.tag` line in each env file is written by CI (dev) and the `promote` workflow (qa/prod). Don't edit it by hand.
- **Adding a service** = create a new folder with these files. The Argo CD ApplicationSet (`argocd/applicationset-services.yaml`)
  discovers `deploy/services/*/*` and creates the apps `<service>-dev|qa|prod` automatically (Git *directory generator*).
- Currently: `auth/` (skeleton: `/api/auth/version`, health probes, Postgres connection).
