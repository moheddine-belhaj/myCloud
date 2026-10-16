// Package httpserver runs an http.Server until its context is cancelled and
// then shuts it down gracefully. Every MyCloud service uses it, so shutdown
// behaviour is identical everywhere.
package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Run starts srv and blocks until ctx is cancelled (SIGINT/SIGTERM) or the
// server fails. On cancellation it stops accepting new connections and waits
// up to shutdownTimeout for in-flight requests to finish.
func Run(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error {
	// ListenAndServe blocks, so run it in a goroutine and report its result on a channel.
	// Buffered (size 1) so the goroutine can send and exit even if nobody reads.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	// Wait for whichever happens first: a shutdown signal or a server failure.
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	// Shutdown stops accepting new connections and waits for in-flight requests.
	return srv.Shutdown(shutdownCtx)
}