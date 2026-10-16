// Package database opens the PostgreSQL connection pool and exposes a
// readiness check for it.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a connection pool for url.
//
// pgxpool connects lazily: no connection is made here. The API therefore starts
// even while Postgres is still booting, and /readyz keeps the pod out of the
// Service until the database answers. Never log url: it contains the password.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	// Small pool: max_connections is 50 per database cluster, shared by
	// every replica and, later, every service.
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute // recycle so failovers and DNS changes are picked up
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return pool, nil
}

// Checker adapts a pool to health.Checker so /readyz can ping the database.
type Checker struct {
	Pool *pgxpool.Pool
}

// Name is the key shown in the /readyz response.
func (Checker) Name() string { return "postgres" }

// Check acquires a connection and runs a round trip to the server.
func (c Checker) Check(ctx context.Context) error {
	return c.Pool.Ping(ctx)
}