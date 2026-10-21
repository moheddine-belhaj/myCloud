// Command auth runs the MyCloud authentication service.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/config"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/migrations"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/password"
	"github.com/moheddine-belhaj/mycloud/backend/internal/database"
	"github.com/moheddine-belhaj/mycloud/backend/internal/health"
	"github.com/moheddine-belhaj/mycloud/backend/internal/httpserver"
)

// version is set at build time: -ldflags "-X main.version=sha-abc1234".
var version = "dev"

// maxConcurrentHashes limits parallel argon2id runs: 4 x 19 MiB fits the
// pod's 256 MiB memory limit with room for everything else.
const maxConcurrentHashes = 4

const usage = `usage:
  auth [serve]                                  start the HTTP server
  auth create-admin --email EMAIL < password    create an admin account
  auth create-invite [--ttl 168h]               print a new invite code
`

// newMux builds the router. Public routes live under /api/auth/, because the
// Ingress sends that path prefix to this service. /healthz and /readyz are
// only used by the kubelet inside the cluster. api may be nil in tests that
// only need the probes.
func newMux(api *auth.Handler, checks ...health.Checker) *http.ServeMux {
	mux := http.NewServeMux()
	health.New(checks...).Register(mux)
	mux.HandleFunc("GET /api/auth/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "auth", "version": version})
	})
	if api != nil {
		api.Routes(mux)
	}
	return mux
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		slog.Error("auth failed", "err", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		slog.SetDefault(slog.New(slog.NewJSONHandler(stdout, nil)).With("service", "auth"))
		return serve()
	case "create-admin", "create-invite":
		// Logs go to stderr so stdout only carries the command's result.
		slog.SetDefault(slog.New(slog.NewJSONHandler(stderr, nil)).With("service", "auth", "command", cmd))
		return runCLI(cmd, args, stdin, stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func serve() error {
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

	// Apply the schema in the background: the server starts right away
	// (liveness OK) and /readyz reports "migrations: fail" until it is done.
	migrator := database.NewMigrator(pool, migrations.FS)
	go migrator.RunWithRetry(ctx)

	svc := auth.NewService(pool, password.NewHasher(password.DefaultParams, maxConcurrentHashes))
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newMux(auth.NewHandler(svc), database.Checker{Pool: pool}, migrator),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("listening", "addr", cfg.Addr, "version", version)
	return httpserver.Run(ctx, srv, cfg.ShutdownTimeout)
}
