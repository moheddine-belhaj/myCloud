package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestLoadTOTPKeys(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	good := base64.StdEncoding.EncodeToString(key)

	for name, tc := range map[string]struct {
		value   string
		wantErr string
	}{
		"valid":            {good, ""},
		"trailing newline": {good + "\n", ""},
		"missing":          {"", "TOTP_KEY is required"},
		"not base64":       {"%%%", "not valid base64"},
		"16 bytes":         {base64.StdEncoding.EncodeToString(key[:16]), "must be 32 bytes"},
	} {
		t.Run(name, func(t *testing.T) {
			kr, err := loadTOTPKeys(func(k string) string {
				if k == "TOTP_KEY" {
					return tc.value
				}
				return ""
			})
			if tc.wantErr == "" {
				if err != nil || kr.CurrentID() != totpKeyID {
					t.Fatalf("got %v, %v; want key ID %d", kr, err, totpKeyID)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
			if strings.Contains(err.Error(), good) {
				t.Error("error message contains the key")
			}
		})
	}
}
