# api

- `base/` – manifests shared by all environments.
- `components/network-policy/` – NetworkPolicy: only Traefik may reach the api pods. Included per overlay (dev first, see `../baseline/README.md`).
- `overlays/` – per-environment changes (`dev`, `qa`, `prod`): image tag, namespace, hosts, middleware names.
