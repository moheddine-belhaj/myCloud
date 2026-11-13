package auth

import (
	"errors"
	"net/http"
)

// Identity headers set by Verify. Traefik copies them onto the request it
// passes to the protected service (ForwardAuth "authResponseHeaders"), so a
// service behind it knows who is calling without touching the session table.
const (
	HeaderUserID   = "X-User-Id"
	HeaderUserRole = "X-User-Role"
)

// Verify is the ForwardAuth endpoint (M4). Traefik calls it before every
// request to a protected service, with the original request's headers
// (cookies included) and X-Forwarded-Method/-Host describing that request.
//
//   - valid session: 200 with X-User-Id and X-User-Role, no body
//   - no or expired session: 401 {"error":"unauthenticated"}; Traefik returns
//     this response to the client and the service never sees the request
//   - cross-site POST/PUT/PATCH/DELETE: 403 {"error":"cross_origin"}, so
//     every service behind Traefik gets the same CSRF protection as auth
//   - anything else (database down): 500, so access fails closed
//
// X-User-Role is the effective role: "admin" only with 2FA on, the same rule
// as RequireAdmin, so no service can grant admin rights to a password alone.
//
// It is served on the internal listener only (INTERNAL_ADDR, port 8081): the
// public Ingress never routes to it.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if err := crossOrigin.Check(forwardedRequest(r)); err != nil {
		writeError(w, http.StatusForbidden, "cross_origin", "Cross-origin requests are not allowed.")
		return
	}

	u, err := h.svc.UserBySession(r.Context(), sessionToken(r))
	if errors.Is(err, ErrNoSession) {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Log in first.")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	role := RoleUser
	if u.Role == RoleAdmin && u.TOTPEnabled {
		role = RoleAdmin
	}
	w.Header().Set(HeaderUserID, u.ID.String())
	w.Header().Set(HeaderUserRole, role)
	w.WriteHeader(http.StatusOK)
}

// crossOrigin is the same check Protect uses, run here against the request
// Traefik is about to forward instead of the verify call itself.
var crossOrigin = http.NewCrossOriginProtection()

// forwardedRequest describes the original client request: Traefik always
// calls Verify with GET and puts the real method and host in X-Forwarded-*.
// Without those headers (curl, tests) the verify request itself is used.
func forwardedRequest(r *http.Request) *http.Request {
	fr := r.Clone(r.Context())
	if m := r.Header.Get("X-Forwarded-Method"); m != "" {
		fr.Method = m
	}
	if host := r.Header.Get("X-Forwarded-Host"); host != "" {
		fr.Host = host
	}
	return fr
}
