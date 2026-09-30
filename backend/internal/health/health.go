// Package health serves the liveness (/healthz) and readiness (/readyz) probes.
//
// Liveness answers "is the process alive?" and never checks dependencies, so a
// database outage doesn't make Kubernetes restart healthy pods.
// Readiness answers "can this pod serve traffic?" and runs every registered check.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Checker reports whether one dependency is usable. A Postgres pool, Redis
// client, etc. can be wrapped to satisfy it later.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// Handler serves the health endpoints.
type Handler struct {
	checks  []Checker
	timeout time.Duration
}

// New returns a Handler that runs the given checks on /readyz.
// `checks ...Checker` is a variadic parameter: callers pass zero or more checkers.
func New(checks ...Checker) *Handler {
	return &Handler{checks: checks, timeout: 2 * time.Second}
}

// Register adds the health routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Go 1.22+ patterns: the method prefix makes the mux reply 405 to other methods.
	mux.HandleFunc("GET /healthz", h.liveness)
	mux.HandleFunc("GET /readyz", h.readiness)
}

type response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (h *Handler) liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, response{Status: "ok"})
}

func (h *Handler) readiness(w http.ResponseWriter, r *http.Request) {
	// Derive from the request context so checks stop if the client disconnects,
	// and cap them so a hung dependency can't hang the probe.
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp := response{Status: "ok", Checks: map[string]string{}}
	status := http.StatusOK
	for _, c := range h.checks {
		if err := c.Check(ctx); err != nil {
			// Don't expose the error text: this endpoint may be reachable
			// through Traefik. Details belong in the logs.
			resp.Checks[c.Name()] = "fail"
			resp.Status = "unavailable"
			status = http.StatusServiceUnavailable
			continue
		}
		resp.Checks[c.Name()] = "ok"
	}
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // headers are already sent; nothing useful to do on error
}
