# MyCloud

A self-hosted, multi-tenant personal cloud running on two mini-PCs at home.

Users join with an invite code, sign in with a password plus two-factor authentication (TOTP), and
create up to 5 **spaces** to upload, preview and download images, videos and files. Each account has
a **15 GB** storage quota shared across its spaces.

The project is also a hands-on DevOps platform: Go services, Kubernetes, GitOps and a layered security
setup, all built and run from a home lab.

> **Status:** early development. The CI/CD pipeline, a minimal Go API (`/healthz`, `/readyz`,
> `/version`), CloudNativePG Postgres, Vault-backed secrets and the first part of the auth service
> (invite registration, login, logout, sessions, admin invites, TOTP 2FA with recovery codes) are in place.
> ForwardAuth (trusted identity at the edge) is done; a per-environment Redis is deployed and
> three-layer rate limiting (Cloudflare, Traefik, auth + Redis) is in progress. Features are added phase by phase (see [Roadmap](#roadmap)).

## Tech stack

| Area | Technology |
|------|------------|
| Backend | Go (stdlib `net/http`, `log/slog`), pgx + sqlc, goose migrations, OpenAPI (oapi-codegen) |
| Frontend | React + TypeScript (Vite), TanStack Query, React Router, Uppy for resumable uploads |
| Database | PostgreSQL (CloudNativePG operator) for metadata |
| File storage | Garage (S3-compatible object storage) |
| Cache / rate limits | Redis (own Helm chart, ACL user, no persistence), used for login rate-limit counters; sessions later |
| Events | NATS JetStream |
| Uploads | tus protocol (resumable, chunked uploads) |
| File processing | ClamAV (malware scan), libvips (images), FFmpeg (video), pdftoppm (PDF) |
| Containers | Docker, distroless base images |
| Orchestration | k3s (lightweight Kubernetes), Traefik ingress |
| CI | GitHub Actions |
| Registry | GitHub Container Registry (GHCR, private) |
| CD | Argo CD + Kustomize (GitOps) |
| Secrets | HashiCorp Vault (self-hosted on a VPS) + External Secrets Operator |
| Networking | Cloudflare (DNS, WAF, Tunnel, rate-limit rule), Tailscale (admin access) |
| Edge auth / limits | Traefik ForwardAuth, `RateLimit` and strip-identity middlewares, NetworkPolicies |
| Observability (planned) | Prometheus, Grafana, Loki, Tempo, OpenTelemetry |

## Architecture

```mermaid
flowchart LR
    U[Browser] -->|HTTPS| CF[Cloudflare<br/>DNS · WAF · DDoS]
    CF -->|Cloudflare Tunnel| T[Traefik]
    subgraph k3s["k3s cluster (cp1 + worker1)"]
        T --> FE[Frontend]
        T --> API[Go API]
        API --> PG[(PostgreSQL)]
        API --> S3[(Garage S3)]
        API --> R[(Redis<br/>rate limits)]
        API --> N{{NATS}}
        N --> W[Workers<br/>scan · media · docs]
        W --> S3
    end
    ADMIN[Admin] -->|Tailscale| k3s
```

- **Single origin:** everything is served from one domain. `/` is the frontend and `/api/*` goes to the
  backend, so there is no CORS and sessions use secure cookies.
- **Files** are stored in object storage under ID-based keys, never under user-supplied names.
- **Uploads** are split into chunks below Cloudflare's 100 MB request limit and can resume after an
  interruption.
- **Rate limiting in three layers:** a Cloudflare rule on login/register, a Traefik `RateLimit`
  middleware per client IP, and failed-login counters (per account+IP, per IP, per account) in Redis
  inside the auth service. Redis is reachable only from auth pods and the limiter fails open if Redis is down.
- **Every upload is scanned** for malware before it becomes available.

## Project structure

```
myCloud/
├── backend/                   Go API
│   ├── cmd/                   entry points: api/, auth/
│   ├── config/                configuration from environment variables
│   ├── internal/
│   │   ├── health/            /healthz and /readyz probes
│   │   ├── auth/              register, login, sessions, TOTP 2FA
│   │   ├── identity/          trusted X-User-Id from ForwardAuth
│   │   ├── user/              user accounts               (planned)
│   │   ├── space/             spaces and quota            (planned)
│   │   ├── file/              file metadata and downloads (planned)
│   │   └── storage/           S3 object storage client    (planned)
│   ├── migrations/            database migrations
│   ├── tests/                 integration tests
│   └── Dockerfile
├── deploy/
│   ├── apps/                  Kustomize apps: api/, postgres/ (base + dev/qa/prod overlays)
│   ├── charts/                Helm charts: service/ (generic, Go services), redis/
│   ├── data/                  per-env values for data services with their own chart (redis/)
│   ├── services/              per-service, per-env Helm values (auth/)
│   ├── argocd/                Argo CD install, projects, ApplicationSets (apps, services, data)
│   └── platform/              cloudflared tunnel, External Secrets store
├── infra/                     Ansible: Vault on a VPS (config, snapshots)
└── .github/workflows/
    ├── backend.yml            test → validate manifests → build → deploy to dev
    └── promote.yml            promote an image dev → qa → prod
```

## Deployment

### Infrastructure

MyCloud runs on a **two-node k3s cluster** of Fujitsu Esprimo Q958 mini-PCs at home:

| Node | Role |
|------|------|
| `cp1` | Kubernetes control plane (also runs workloads) |
| `worker1` | Worker node |

![MyCloud architecture](docs/architecture/preview_infra.png)


- The home connection has **no public IPv4 and no open ports**. Public traffic reaches the cluster only
  through a **Cloudflare Tunnel**, at `cloud.mohedine.dev`.
- Cluster administration (`kubectl`, SSH) is possible **only over Tailscale**.

### Environments

All three environments run in the same cluster, each in its own namespace:

| Environment | Namespace | How it gets updated |
|-------------|-----------|---------------------|
| dev | `mycloud-dev` | Automatically on every push to `main` |
| qa | `mycloud-qa` | Manually, by promoting the image running in dev |
| prod | `mycloud-prod` | Manually, by promoting the image running in qa (requires approval) |

### CI/CD pipeline

```
push to main ─► test (go vet, go test -race)
             ─► build image ghcr.io/moheddine-belhaj/mycloud-api:sha-<commit>
             ─► commit the new tag to deploy/apps/api/overlays/dev
             ─► Argo CD syncs mycloud-dev

Actions ▸ promote ▸ qa    ─► copy dev's tag to the qa overlay   ─► Argo CD syncs mycloud-qa
Actions ▸ promote ▸ prod  ─► copy qa's tag to the prod overlay  ─► Argo CD syncs mycloud-prod
```

Each image is **built once and promoted**: prod runs the exact image that was tested in dev and qa.
Git is the single source of truth. Argo CD watches `main` and applies whatever the overlays say.

## Run locally

Requires Go 1.26+ (and Docker for the container build).

```bash
cd backend
go test -race ./...
go run ./cmd/api              # listens on :8080 (override with ADDR)

curl localhost:8080/healthz   # {"status":"ok"}
curl localhost:8080/version   # {"version":"dev"}
```

With Docker:

```bash
cd backend
docker build --build-arg VERSION=local -t mycloud-api:local .
docker run --rm -p 8080:8080 mycloud-api:local
```

## Roadmap

| # | Phase | Status |
|---|-------|--------|
| 0 | Platform: k3s cluster, Traefik, Cloudflare Tunnel, Tailscale | In progress |
| 1 | GitOps: CI, GHCR, Argo CD, dev/qa/prod | In progress |
| 2 | Data layer: PostgreSQL, Redis (done, per env), Garage, NATS, backups | In progress |
| 3 | Auth: invite registration, login, 2FA, ForwardAuth | In progress |
| 4 | Rate limiting: Redis, Traefik, Cloudflare, auth lockouts | In progress |
| 5 | Spaces and quotas | Planned |
| 6 | File upload, listing and download | Planned |
| 7 | Processing pipeline: malware scan, thumbnails, previews | Planned |
| 8 | Security hardening | Planned |
| 9 | Observability: metrics, logs, tracing | Planned |
| 10 | Extras: share links, search, video streaming | Planned |

## Author

Mohedine Belhaj
