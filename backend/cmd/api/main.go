// Command api runs the MyCloud backend HTTP server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/config"
	"github.com/moheddine-belhaj/mycloud/backend/internal/health"
)

// version is overwritten at build time by the linker:
//
//	go build -ldflags "-X main.version=sha-abc1234" ./cmd/api
//
// -X can only set package-level string variables, which is why this is a var and not a const.
var version = "dev"

// newMux builds the router. It is separate from run so tests can exercise
// the real routes without starting a server.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	health.New().Register(mux) // add DB/Redis checkers here once they exist
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	})
	return mux
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// run holds the real startup logic and returns errors instead of calling
// os.Exit so main can decide how to fail. This makes it easier to test.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// ctx is cancelled when the process receives SIGINT (Ctrl+C) or SIGTERM (Kubernetes stop).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newMux(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ListenAndServe blocks, so run it in a goroutine and report its result on a channel.
	// Buffered (size 1) so the goroutine can send and exit even if nobody reads.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "version", version)
		errCh <- srv.ListenAndServe()
	}()

	// select waits for whichever happens first: a shutdown signal or a server failure.
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	// Shutdown stops accepting new connections and waits for in-flight requests.
	return srv.Shutdown(shutdownCtx)
}
