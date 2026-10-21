package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/migrations"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/password"
	"github.com/moheddine-belhaj/mycloud/backend/internal/database"
	"github.com/moheddine-belhaj/mycloud/backend/internal/testdb"
)

const goodPassword = "correct horse battery"

// newTestService returns a Service on a fresh, migrated database.
// Hashing uses cheap parameters so the tests stay fast.
func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.New(t)
	if err := database.NewMigrator(pool, migrations.FS).Up(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cheap := password.Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	return NewService(pool, password.NewHasher(cheap, 4)), pool
}

func newInvite(t *testing.T, s *Service) string {
	t.Helper()
	inv, err := s.CreateInvite(context.Background(), nil, DefaultInviteTTL)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	return inv.Code
}

func TestRegister(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	code := newInvite(t, s)

	u, err := s.Register(ctx, " "+code+"\n", "  Mo@Example.COM ", goodPassword)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.Email != "mo@example.com" || u.Role != RoleUser {
		t.Errorf("user = %+v, want email mo@example.com, role user", u)
	}

	// The password is stored as an argon2id hash, never in plain text.
	var hash string
	if err := pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", u.ID).Scan(&hash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if ok, err := s.hasher.Verify(ctx, goodPassword, hash); err != nil || !ok {
		t.Errorf("stored hash does not verify: ok=%v err=%v", ok, err)
	}

	// The invite is used, and by this user.
	var usedBy *string
	if err := pool.QueryRow(ctx, "SELECT used_by::text FROM invites WHERE used_at IS NOT NULL").Scan(&usedBy); err != nil {
		t.Fatalf("read invite: %v", err)
	}
	if usedBy == nil || *usedBy != u.ID.String() {
		t.Errorf("invite used_by = %v, want %s", usedBy, u.ID)
	}

	// A used invite can't be used again.
	if _, err := s.Register(ctx, code, "other@example.com", goodPassword); !errors.Is(err, ErrInvalidInvite) {
		t.Errorf("Register with used invite: err = %v, want ErrInvalidInvite", err)
	}
}

func TestRegisterInvalidInvite(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	if _, err := s.Register(ctx, "no-such-code", "mo@example.com", goodPassword); !errors.Is(err, ErrInvalidInvite) {
		t.Errorf("unknown code: err = %v, want ErrInvalidInvite", err)
	}
	if _, err := s.Register(ctx, "   ", "mo@example.com", goodPassword); !errors.Is(err, ErrInvalidInvite) {
		t.Errorf("empty code: err = %v, want ErrInvalidInvite", err)
	}

	code := newInvite(t, s)
	if _, err := pool.Exec(ctx, "UPDATE invites SET expires_at = now() - interval '1 minute'"); err != nil {
		t.Fatalf("expire invite: %v", err)
	}
	if _, err := s.Register(ctx, code, "mo@example.com", goodPassword); !errors.Is(err, ErrInvalidInvite) {
		t.Errorf("expired code: err = %v, want ErrInvalidInvite", err)
	}
}

func TestRegisterDuplicateEmailKeepsInvite(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	if _, err := s.Register(ctx, newInvite(t, s), "mo@example.com", goodPassword); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	code := newInvite(t, s)
	if _, err := s.Register(ctx, code, "MO@example.com", goodPassword); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email: err = %v, want ErrEmailTaken", err)
	}
	// The transaction rolled back, so the second invite is still unused.
	var unused int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invites WHERE used_at IS NULL").Scan(&unused); err != nil {
		t.Fatalf("count invites: %v", err)
	}
	if unused != 1 {
		t.Errorf("unused invites = %d, want 1", unused)
	}
	if _, err := s.Register(ctx, code, "new@example.com", goodPassword); err != nil {
		t.Errorf("reuse the kept invite: %v", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	code := newInvite(t, s)

	for _, email := range []string{"", "mo", "mo@", "@example.com", "Mo <mo@example.com>", "mo@localhost", "a b@example.com"} {
		if _, err := s.Register(ctx, code, email, goodPassword); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("email %q: err = %v, want ErrInvalidEmail", email, err)
		}
	}
	if _, err := s.Register(ctx, code, "mo@example.com", "too short"); !errors.Is(err, ErrPasswordLength) {
		t.Errorf("short password: err = %v, want ErrPasswordLength", err)
	}
	// Validation failures must not consume the invite.
	if _, err := s.Register(ctx, code, "mo@example.com", goodPassword); err != nil {
		t.Errorf("Register after failed attempts: %v", err)
	}
}

func TestCreateInvite(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()

	before := time.Now()
	inv, err := s.CreateInvite(ctx, nil, 48*time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if len(inv.Code) != 43 {
		t.Errorf("code length = %d, want 43", len(inv.Code))
	}
	if d := inv.ExpiresAt.Sub(before); d < 47*time.Hour || d > 49*time.Hour {
		t.Errorf("expires in %s, want ~48h", d)
	}

	// Only the hash is stored.
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invites WHERE code_hash = convert_to($1, 'UTF8')", inv.Code).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Error("the plain invite code is stored in the database")
	}

	for _, ttl := range []time.Duration{0, time.Minute, MaxInviteTTL + time.Hour} {
		if _, err := s.CreateInvite(ctx, nil, ttl); !errors.Is(err, ErrInvalidTTL) {
			t.Errorf("ttl %s: err = %v, want ErrInvalidTTL", ttl, err)
		}
	}
}

func TestCreateAdmin(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	u, err := s.CreateAdmin(ctx, "Admin@Example.com", goodPassword)
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if u.Role != RoleAdmin || u.Email != "admin@example.com" {
		t.Errorf("user = %+v, want admin admin@example.com", u)
	}
	if _, err := s.CreateAdmin(ctx, "admin@example.com", goodPassword); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("second CreateAdmin: err = %v, want ErrEmailTaken", err)
	}
	if _, err := s.CreateAdmin(ctx, "b@example.com", "short"); !errors.Is(err, ErrPasswordLength) {
		t.Errorf("short password: err = %v, want ErrPasswordLength", err)
	}
}

func TestRegisterConcurrentSameInvite(t *testing.T) {
	s, _ := newTestService(t)
	code := newInvite(t, s)

	const n = 8
	errs := make(chan error, n)
	for i := range n {
		go func() {
			_, err := s.Register(context.Background(), code, "user"+string(rune('a'+i))+"@example.com", goodPassword)
			errs <- err
		}()
	}
	ok := 0
	for range n {
		err := <-errs
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, ErrInvalidInvite):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d registrations succeeded with one invite, want exactly 1", ok)
	}
}
