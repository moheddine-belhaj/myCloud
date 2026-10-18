package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrator applies a service's embedded goose migrations and reports, as a
// health.Checker, whether they have been applied. Until then /readyz fails,
// so Kubernetes sends no traffic to a pod whose schema is not ready.
type Migrator struct {
	pool *pgxpool.Pool
	fsys fs.FS
	done atomic.Bool
}

// NewMigrator returns a Migrator for the migrations in fsys.
func NewMigrator(pool *pgxpool.Pool, fsys fs.FS) *Migrator {
	return &Migrator{pool: pool, fsys: fsys}
}

// Up applies all pending migrations once. It holds a Postgres advisory lock
// while migrating, so when several replicas start together only one of them
// migrates and the others wait, then find nothing left to do.
func (m *Migrator) Up(ctx context.Context) error {
	// goose works with database/sql; this wraps our pgx pool without opening
	// extra connections. Closing it does not close the pool.
	db := stdlib.OpenDBFromPool(m.pool)
	defer db.Close()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, m.fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	results, err := provider.Up(ctx)
	for _, r := range results {
		if r.Source != nil {
			slog.Info("migration applied", "version", r.Source.Version, "file", path.Base(r.Source.Path), "duration", r.Duration)
		}
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	m.done.Store(true)
	return nil
}

// RunWithRetry calls Up until it succeeds or ctx is cancelled. Start it in a
// goroutine: the HTTP server comes up immediately (liveness passes) while
// readiness waits for the database and the schema.
func (m *Migrator) RunWithRetry(ctx context.Context) {
	backoff := time.Second
	for {
		err := m.Up(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}
		slog.Error("migrations failed, retrying", "err", err, "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// Name is the key shown in the /readyz response.
func (m *Migrator) Name() string { return "migrations" }

// Check reports whether all migrations have been applied.
func (m *Migrator) Check(context.Context) error {
	if m.done.Load() {
		return nil
	}
	return errors.New("migrations not applied yet")
}
