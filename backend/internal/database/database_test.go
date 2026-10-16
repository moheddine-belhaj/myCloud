package database

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenRejectsInvalidURL(t *testing.T) {
	if _, err := Open(context.Background(), "not a url ::"); err == nil {
		t.Fatal("Open with an invalid URL returned no error")
	}
}

func TestCheckFailsWhenDatabaseUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Port 1 on localhost: nothing listens there, so the dial fails fast.
	pool, err := Open(ctx, "postgres://u:p@127.0.0.1:1/db?connect_timeout=1&sslmode=disable")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if err := (Checker{Pool: pool}).Check(ctx); err == nil {
		t.Fatal("Check succeeded against an unreachable database")
	}
}

// TestCheckAgainstPostgres runs only when TEST_DATABASE_URL is set
// (in CI via a Postgres service container, locally via docker run).
func TestCheckAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if err := (Checker{Pool: pool}).Check(ctx); err != nil {
		t.Fatalf("Check: %v", err)
	}
}