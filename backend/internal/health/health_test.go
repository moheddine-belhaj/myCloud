package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeChecker struct {
	name string
	err  error
}

func (f fakeChecker) Name() string                { return f.name }
func (f fakeChecker) Check(context.Context) error { return f.err }

func serve(h *Handler, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		checks     []Checker
		wantCode   int
		wantStatus string
	}{
		{"no checks", nil, http.StatusOK, "ok"},
		{"all pass", []Checker{fakeChecker{"postgres", nil}}, http.StatusOK, "ok"},
		{"one fails", []Checker{fakeChecker{"postgres", errors.New("down")}}, http.StatusServiceUnavailable, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(New(tt.checks...), "/readyz")
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			var body response
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, tt.wantStatus)
			}
		})
	}
}

// Liveness must never depend on the database: a DB outage must not make
// Kubernetes restart otherwise healthy pods.
func TestLivenessIgnoresChecks(t *testing.T) {
	rec := serve(New(fakeChecker{"postgres", errors.New("down")}), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}
