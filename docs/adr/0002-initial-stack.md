# 0002. Initial technology stack

- Status: Accepted
- Date: 2026-09-30
- Phase: 0 (Platform)

## Context

MyCloud is a multi-tenant personal cloud (users, 2FA, spaces, a 15 GB quota per account, file, image
and video upload). The stack has to fit these constraints:

- **One Fujitsu Q958 mini-PC** running single-node k3s, so RAM and CPU are scarce.
- **IPv6-only DS-Lite connection**: no inbound ports. Public traffic comes only through a Cloudflare
  Tunnel, and admin access only through Tailscale.
- **Cloudflare free plan caps request bodies at 100 MB**, but users must be able to upload multi-GB
  videos.
- It is a learning project for Go, microservices and Kubernetes, so standard, well-documented tools
  are preferred over clever ones.

The full stack is listed in the architecture doc (`docs/MyCloudArchitecture&BuildPlan.pdf`, "Tech map").
This ADR records the choices that had real alternatives.

## Decision

1. **Backend in Go** (stdlib `net/http` 1.22+ routing, pgx + sqlc, goose, oapi-codegen, slog, OTel),
   split into seven services (auth, space, upload, file, and scan-, media- and doc-workers), each owning
   its own database. **Frontend** in React + TypeScript + Vite + TanStack Query + Uppy.
2. **Garage for object storage, not MinIO.** Garage is a small S3-compatible store built for self-hosted
   clusters on modest hardware. It uses little memory and can grow from one node to several when the
   second node arrives. MinIO's community edition was cut back heavily in 2025 (admin console
   removed from the community build, community binaries and images no longer published, and the
   project then put into maintenance mode). The architecture doc asked for this to be checked, and it
   rules MinIO out as a long-term dependency.
3. **NATS JetStream for events, not Redis queues.** The upload → scan → process pipeline needs durable
   storage, per-consumer acknowledgements, redelivery with backoff, a max-delivery limit, deduplication
   by message ID, and replay. JetStream provides all of these with durable consumers. Redis stays
   limited to sessions, rate limits and cache, where losing data is acceptable.
4. **tus protocol for uploads** (tusd Go library + Uppy). Chunks stay well under the 100 MB Cloudflare
   limit, interrupted uploads resume, and upload-service streams chunks straight into S3 multipart
   without buffering whole files on disk.
5. **Single-origin routing.** Everything is served from `cloud.mohedine.dev`: `/` for the SPA and
   `/api/auth`, `/api/spaces`, `/api/uploads` and `/api/files` routed by Traefik. This means no CORS,
   and the session can be a `HttpOnly; Secure; SameSite=Strict` cookie, so no tokens are kept in the
   browser. Traefik ForwardAuth checks the session once and passes `X-User-Id` to the services.
6. **Invite-only registration.** New accounts need a single-use invite code. The server's capacity
   (disk, CPU, bandwidth) is personal and finite, so open sign-up would invite abuse, spam accounts and
   storage exhaustion. Invites also shrink the attack surface for credential stuffing and enumeration.
7. **Platform:** k3s + Traefik + cloudflared, Postgres via CloudNativePG, Argo CD + Kustomize for
   GitOps, SOPS + age for secrets in Git, GitHub Actions + GHCR for CI. Observability
   (Prometheus/Loki/Tempo) is deferred to phase 8 to save RAM.

## Alternatives considered

- **MinIO:** the most familiar S3 server, rejected for the community-edition changes above. **SeaweedFS:**
  capable and fast, but has more moving parts (master, volume and filer servers) than one small node
  needs. It is the fallback if Garage's S3 coverage turns out to be insufficient. **Local filesystem / PVC:** no S3
  API, harder to scale out, and ties files to one node's disk.
- **Redis Streams / lists as a queue:** fewer components, but redelivery, dead-lettering and
  deduplication would have to be built by hand, and Redis durability is tuned for cache use. **RabbitMQ:**
  capable, but heavier on memory. **Direct HTTP calls between services:** simple, but couples service
  availability and loses work on crashes.
- **Plain multipart POST uploads:** break the 100 MB edge limit and cannot resume. **Presigned S3 uploads
  from the browser:** would require exposing the object store publicly and skipping server-side quota
  checks per chunk.
- **Separate subdomains per API:** needs CORS and cross-site cookie handling, which adds attack surface
  for no benefit.
- **Open registration with email verification only:** too easy to abuse on a single home server.

## Consequences

- Garage covers a narrower part of the S3 API than MinIO/AWS (for example no object versioning or
  object lock). Features that need these require a new ADR.
- NATS is one more stateful component to run, back up and monitor, at a few tens of MB of RAM.
- tus adds protocol state: abandoned uploads must be expired and their reserved quota released by a
  cleanup job.
- Single-origin routing makes Traefik and ForwardAuth central. Their misconfiguration is a
  high-impact risk and is covered by the `security-review` skill.
- Invites need an admin flow to create them, and onboarding a friend takes a manual step.
- Seven services on one node make resource limits and a lean base image (distroless) mandatory from the
  start.
