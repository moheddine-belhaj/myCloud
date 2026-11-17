package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testID = "d81da0aa-bf24-49e3-b8fb-6a8c3292c139"

func request(id, role string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if id != "" {
		r.Header.Set(HeaderUserID, id)
	}
	if role != "" {
		r.Header.Set(HeaderUserRole, role)
	}
	return r
}

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name, id, role string
		ok             bool
	}{
		{"user", testID, RoleUser, true},
		{"admin", testID, RoleAdmin, true},
		{"no headers", "", "", false},
		{"no role", testID, "", false},
		{"unknown role", testID, "root", false},
		{"id not a uuid", "42", RoleUser, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := FromRequest(request(tc.id, tc.role))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && (got.UserID != tc.id || got.Role != tc.role) {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestRequire(t *testing.T) {
	var seen Identity
	h := Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, request(testID, RoleUser))
	if rec.Code != http.StatusNoContent || seen.UserID != testID || seen.Role != RoleUser {
		t.Fatalf("with identity: status %d, seen %+v", rec.Code, seen)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, request("", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without identity: status %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}
