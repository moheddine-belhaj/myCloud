// Command auth runs the MyCloud authentication service.

package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/keyring"
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

// sessionCleanupEvery is how often expired sessions are deleted.
const sessionCleanupEvery = time.Hour

// totpKeyID names the key in TOTP_KEY. A future key rotation adds key 2 and
// keeps key 1 for opening older rows.
const totpKeyID = 1

// loadTOTPKeys reads the AES-256 key that encrypts TOTP seeds (base64 in
// TOTP_KEY, synced from Vault). The service refuses to start without a valid
// key: running without one would break 2FA silently.
func loadTOTPKeys(getenv func(string) string) (*keyring.Keyring, error) {
	v := getenv("TOTP_KEY")
	if v == "" {
		return nil, errors.New("TOTP_KEY is required")
	}
	kr, err := keyring.Parse(totpKeyID, v)
	if err != nil {
		return nil, fmt.Errorf("TOTP_KEY: %w", err)
	}
	return kr, nil
}

// newMux builds the router. Public routes live under /api/auth/, because the
// Ingress sends that path prefix to this service. /healthz and /readyz are
// only used by the kubelet inside the cluster. api may be nil in tests that
// only need the probes.
//
// Every route sits behind auth.Protect, which rejects cross-site POSTs.
func newMux(api *auth.Handler, checks ...health.Checker) http.Handler {
	mux := http.NewServeMux()
	health.New(checks...).Register(mux)
	mux.HandleFunc("GET /api/auth/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "auth", "version": version})
	})
	if api != nil {
		api.Routes(mux)
	}
	return auth.Protect(mux)
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
	totpKeys, err := loadTOTPKeys(os.Getenv)
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

	svc := auth.NewService(pool, password.NewHasher(password.DefaultParams, maxConcurrentHashes))

	// Apply the schema in the background: the server starts right away
	// (liveness OK) and /readyz reports "migrations: fail" until it is done.
	// Session cleanup starts once the tables exist and runs until shutdown.
	migrator := database.NewMigrator(pool, migrations.FS)
	go func() {
		migrator.RunWithRetry(ctx)
		svc.RunSessionCleanup(ctx, sessionCleanupEvery)
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newMux(auth.NewHandler(svc), database.Checker{Pool: pool}, migrator),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	// Only the key ID is logged, never the key.
	slog.Info("listening", "addr", cfg.Addr, "version", version, "totp_key_id", totpKeys.CurrentID())
	return httpserver.Run(ctx, srv, cfg.ShutdownTimeout)
}
