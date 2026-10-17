// Command api runs the MyCloud backend HTTP server.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/config"
	"github.com/moheddine-belhaj/mycloud/backend/internal/database"
	"github.com/moheddine-belhaj/mycloud/backend/internal/health"
	"github.com/moheddine-belhaj/mycloud/backend/internal/httpserver"
)

// version is overwritten at build time by the linker:
//
//	go build -ldflags "-X main.version=sha-abc1234" ./cmd/api
//
// -X can only set package-level string variables, which is why this is a var and not a const.
var version = "dev"

// newMux builds the router. checks are run by /readyz. It is separate from run
// so tests can exercise the real routes without starting a server.
func newMux(checks ...health.Checker) *http.ServeMux {
	mux := http.NewServeMux()
	health.New(checks...).Register(mux)
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	})
	return mux
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// run holds the real startup logic and returns errors instead of calling
// os.Exit so main can decide how to fail.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// ctx is cancelled when the process receives SIGINT (Ctrl+C) or SIGTERM (Kubernetes stop).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// The pool connects lazily: startup does not wait for Postgres,
	// /readyz reports "postgres: fail" until the database answers.
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close() // runs after the server has drained in-flight requests

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newMux(database.Checker{Pool: pool}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("listening", "addr", cfg.Addr, "version", version)
	return httpserver.Run(ctx, srv, cfg.ShutdownTimeout)
}
