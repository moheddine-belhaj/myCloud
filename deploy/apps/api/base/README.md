# base

Common manifests for the API:

- `deployment.yaml` – the pods (image, resources, probes).
- `service.yaml` – internal network address for the pods.
- `ingress.yaml` – routes outside traffic to the service.
- `kustomization.yaml` – lists the files above.
