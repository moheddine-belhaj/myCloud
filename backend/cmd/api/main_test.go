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
