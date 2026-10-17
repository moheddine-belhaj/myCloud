// Command auth runs the MyCloud authentication service.
//
// Milestone 1 is the skeleton: health, readiness (Postgres) and version, all
// deployed through the shared Helm chart. Invites, registration, login,
// sessions and TOTP 2FA are added in the next milestones.
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

// version is set at build time: -ldflags "-X main.version=sha-abc1234".
var version = "dev"

// newMux builds the router. Public routes live under /api/auth/, because the
// Ingress sends that path prefix to this service. /healthz and /readyz are
// only used by the kubelet inside the cluster.
func newMux(checks ...health.Checker) *http.ServeMux {
	mux := http.NewServeMux()
	health.New(checks...).Register(mux)
	mux.HandleFunc("GET /api/auth/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "auth", "version": version})
	})
	return mux
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "auth"))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newMux(database.Checker{Pool: pool}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("listening", "addr", cfg.Addr, "version", version)
	return httpserver.Run(ctx, srv, cfg.ShutdownTimeout)
}
