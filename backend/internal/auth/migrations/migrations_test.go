package migrations_test

import (
	"context"
	"testing"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/migrations"
	"github.com/moheddine-belhaj/mycloud/backend/internal/database"
	"github.com/moheddine-belhaj/mycloud/backend/internal/testdb"
)

func TestMigrationsApplyAndAreIdempotent(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	m := database.NewMigrator(pool, migrations.FS)

	if err := m.Up(ctx); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := m.Up(ctx); err != nil { // nothing pending: must be a no-op
		t.Fatalf("second Up: %v", err)
	}
	if err := m.Check(ctx); err != nil {
		t.Fatalf("Check after Up: %v", err)
	}

	for _, table := range []string{"users", "invites", "sessions"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after migrating", table)
		}
	}
}

func TestSchemaRules(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := database.NewMigrator(pool, migrations.FS).Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Emails must be stored lowercase, and only once.
	if _, err := pool.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES ('Mo@x.com', 'h')`); err == nil {
		t.Error("uppercase email was accepted")
	}
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('mo@x.com', 'h') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES ('mo@x.com', 'h')`); err == nil {
		t.Error("duplicate email was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (email, password_hash, role) VALUES ('a@x.com', 'h', 'root')`); err == nil {
		t.Error("unknown role was accepted")
	}

	// An invite can be claimed exactly once.
	if _, err := pool.Exec(ctx, `INSERT INTO invites (code_hash, expires_at) VALUES ('\x01', now() + interval '1 day')`); err != nil {
		t.Fatalf("insert invite: %v", err)
	}
	claim := `UPDATE invites SET used_at = now(), used_by = $1
	          WHERE code_hash = '\x01' AND used_at IS NULL AND expires_at > now()`
	if tag, err := pool.Exec(ctx, claim, userID); err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("first claim: rows=%d err=%v, want 1 row", tag.RowsAffected(), err)
	}
	if tag, err := pool.Exec(ctx, claim, userID); err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("second claim: rows=%d err=%v, want 0 rows", tag.RowsAffected(), err)
	}

	// Deleting a user deletes their sessions.
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('\x0a', $1, now() + interval '7 days')`, userID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Errorf("sessions after user delete = %d, want 0", sessions)
	}
}
