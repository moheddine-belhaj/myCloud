# base

Common manifests for the API:

- `deployment.yaml` – the pods (image, resources, probes).
- `service.yaml` – internal network address for the pods.
- `ingress.yaml` – routes outside traffic to the service (public: `/version`, health).
- `ingress-protected.yaml` – `/api/*` behind Traefik ForwardAuth (M4): the Middlewares `strip-identity` and `forwardauth` from the auth release run first, so the api only sees requests with a valid session and trusted `X-User-*` headers (`GET /api/whoami` echoes them). Traefik prefers the longest path, so `/api/auth` still goes to auth. The middleware names contain the namespace (`mycloud-<env>-…`): prod is the default, dev and qa patch it in their overlay.
- `kustomization.yaml` – lists the files above.
