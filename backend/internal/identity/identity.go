// Package identity reads the caller identity that Traefik ForwardAuth adds to
// protected requests. See README.md for the trust model.
package identity

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// Headers set by auth /verify and copied by Traefik onto the forwarded request.
const (
	HeaderUserID   = "X-User-Id"
	HeaderUserRole = "X-User-Role"
)

// Roles a request can carry.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Identity is the authenticated caller of a request.
type Identity struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type ctxKey struct{}

// FromRequest returns the identity from the ForwardAuth headers; ok is false if they are missing or malformed.
func FromRequest(r *http.Request) (Identity, bool) {
	id, role := r.Header.Get(HeaderUserID), r.Header.Get(HeaderUserRole)
	if _, err := uuid.Parse(id); err != nil || (role != RoleUser && role != RoleAdmin) {
		return Identity{}, false
	}
	return Identity{UserID: id, Role: role}, true
}

// FromContext returns the identity stored by Require.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Require answers 401 when a request has no valid identity, otherwise stores it in the request context.
func Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := FromRequest(r)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated", "message": "Log in first."})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}
