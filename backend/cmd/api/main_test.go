package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes(t *testing.T) {
	mux := newMux()
	for _, path := range []string{"/healthz", "/readyz", "/version"} {
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
	// Tests live in package main, so they can read and swap the unexported var.
	old := version
	version = "sha-test123"
	t.Cleanup(func() { version = old })

	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["version"] != "sha-test123" {
		t.Errorf("version = %q, want %q", got["version"], "sha-test123")
	}
}

func TestWhoami(t *testing.T) {
	const id = "d81da0aa-bf24-49e3-b8fb-6a8c3292c139"
	tests := []struct {
		name   string
		hdr    map[string]string
		status int
	}{
		{"identity from ForwardAuth", map[string]string{"X-User-Id": id, "X-User-Role": "admin"}, http.StatusOK},
		{"no identity", nil, http.StatusUnauthorized},
		{"malformed identity", map[string]string{"X-User-Id": "1", "X-User-Role": "admin"}, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
			for k, v := range tc.hdr {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			newMux().ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if tc.status != http.StatusOK {
				return
			}
			var got map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got["userId"] != id || got["role"] != "admin" {
				t.Errorf("body = %v", got)
			}
		})
	}
}
