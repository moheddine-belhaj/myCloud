package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes(t *testing.T) {
	mux := newMux(nil)
	for _, path := range []string{"/healthz", "/readyz", "/api/auth/version"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", path, rec.Code)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	old := version
	version = "sha-test123"
	t.Cleanup(func() { version = old })

	rec := httptest.NewRecorder()
	newMux(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/version", nil))

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["service"] != "auth" || got["version"] != "sha-test123" {
		t.Errorf("body = %v, want service=auth version=sha-test123", got)
	}
}

// The router itself is wrapped in cross-origin protection, not only the API
// handler, so a forgotten wrapper on a new route can't open a CSRF hole.
func TestRouterRejectsCrossSitePost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/version", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	newMux(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site POST = %d, want 403", rec.Code)
	}
}
