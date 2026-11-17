# identity

Reads who is calling a protected service. Since M4, services don't check sessions themselves: Traefik asks auth (`GET :8081/verify`) before it forwards a request, and auth answers with two headers that Traefik copies onto the request:

| Header | Value |
|---|---|
| `X-User-Id` | the user's UUID |
| `X-User-Role` | `user` or `admin` (admin only with 2FA on) |

- `FromRequest(r)` – reads and validates both headers (UUID, known role).
- `Require(next)` – middleware: `401 {"error":"unauthenticated"}` without a valid identity, otherwise the identity is in the context.
- `FromContext(ctx)` – the identity inside a handler wrapped by `Require`.

## Why services can trust these headers

The headers are only trustworthy because of three rules that work together:

1. **Traefik strips them first.** The `strip-identity` middleware deletes client-sent `X-User-*` before anything else, so a forged `X-User-Role: admin` never reaches auth or the service.
2. **Traefik sets them only after auth said yes.** The `forwardauth` middleware copies `X-User-Id`/`X-User-Role` from auth's `200` response. A `401`/`403` from auth goes straight back to the client.
3. **Only Traefik can reach the pods.** A NetworkPolicy allows ingress to the service only from the Traefik pods, so nobody inside the cluster can call the service directly with made-up headers.

`Require` still rejects requests without headers. If an Ingress is ever deployed without the middlewares, the service fails closed instead of serving anonymous requests.

Usage in a service:

```go
mux.Handle("GET /api/spaces", identity.Require(http.HandlerFunc(listSpaces)))

func listSpaces(w http.ResponseWriter, r *http.Request) {
	id, _ := identity.FromContext(r.Context())
	// id.UserID, id.Role
}
```
