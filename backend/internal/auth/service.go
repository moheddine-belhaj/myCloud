// Package auth is the business logic of the auth service: invites,
// registration, login and sessions.
//
// It knows nothing about HTTP; http.go translates requests into calls to
// Service and its errors into status codes. The CLI in cmd/auth uses the same
// Service, so both paths apply the same rules.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/password"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/store"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/token"
)

// Roles stored in users.role.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Invite lifetimes.
const (
	DefaultInviteTTL = 7 * 24 * time.Hour
	MaxInviteTTL     = 30 * 24 * time.Hour
)

// SessionTTL is how long a login lasts. It is absolute: activity does not
// extend it.
const SessionTTL = 7 * 24 * time.Hour

// maxUserAgent caps the stored User-Agent header (characters).
const maxUserAgent = 256

// Errors returned to callers. The HTTP layer maps each to a status code.
var (
	ErrInvalidEmail  = errors.New("invalid email address")
	ErrEmailTaken    = errors.New("email is already registered")
	ErrInvalidInvite = errors.New("invite code is invalid, used or expired")
	ErrInvalidTTL    = fmt.Errorf("invite lifetime must be between 1h and %s", MaxInviteTTL)
	// ErrInvalidCredentials covers both "unknown email" and "wrong password",
	// so callers can't tell which emails are registered.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrNoSession means the token is missing, unknown or expired.
	ErrNoSession = errors.New("no valid session")
	// ErrPasswordLength is re-exported so callers only need this package.
	ErrPasswordLength = password.ErrLength
)

// User is the public view of an account. It never carries the password hash.
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Invite is a newly created invite. Code is shown once and never stored.
type Invite struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Session is a new login. Token goes into the cookie and is never stored;
// the database only keeps its SHA-256.
type Session struct {
	Token     string
	ExpiresAt time.Time
	User      User
}

// Service implements the auth use cases on top of the auth database.
type Service struct {
	pool   *pgxpool.Pool
	q      *store.Queries
	hasher *password.Hasher
	// dummyHash is verified against when a login email doesn't exist, so
	// that answer takes as long as a wrong password. Made once.
	dummyHash func() (string, error)
}

// NewService returns a Service using pool for storage and hasher for passwords.
func NewService(pool *pgxpool.Pool, hasher *password.Hasher) *Service {
	s := &Service{
		pool:   pool,
		q:      store.New(pool),
		hasher: hasher,
		dummyHash: sync.OnceValues(func() (string, error) {
			return hasher.Hash(context.Background(), token.New())
		}),
	}
	// Make the dummy hash now, in the background, so even the first failed
	// login doesn't take longer than the others.
	go s.dummyHash()
	return s
}

// CreateInvite stores a new single-use invite valid for ttl and returns its
// code. createdBy is the admin who asked for it, or nil when made by the CLI.
func (s *Service) CreateInvite(ctx context.Context, createdBy *uuid.UUID, ttl time.Duration) (Invite, error) {
	if ttl < time.Hour || ttl > MaxInviteTTL {
		return Invite{}, ErrInvalidTTL
	}
	code := token.New()
	row, err := s.q.CreateInvite(ctx, store.CreateInviteParams{
		CodeHash:  token.Hash(code),
		CreatedBy: createdBy,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(ttl), Valid: true},
	})
	if err != nil {
		return Invite{}, fmt.Errorf("create invite: %w", err)
	}
	by := "cli"
	if createdBy != nil {
		by = createdBy.String()
	}
	slog.InfoContext(ctx, "invite created", "invite_id", row.ID, "created_by", by, "expires_at", row.ExpiresAt.Time)
	return Invite{Code: code, ExpiresAt: row.ExpiresAt.Time}, nil
}

// Register creates a user account with an invite code. The invite is claimed
// and the user inserted in one transaction: either both happen or neither,
// and two people can never use the same code.
func (s *Service) Register(ctx context.Context, inviteCode, email, pw string) (User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	inviteCode = strings.TrimSpace(inviteCode)
	if inviteCode == "" {
		return User{}, ErrInvalidInvite
	}
	if err := password.CheckLength(pw); err != nil {
		return User{}, err
	}
	// Hash before opening the transaction: argon2id takes ~50 ms and a lot of
	// memory, and must not hold a database connection or row lock meanwhile.
	hash, err := s.hasher.Hash(ctx, pw)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	var u User
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		// Claim the invite first, so a caller without a valid code can't
		// learn whether an email is registered.
		inviteID, err := q.ClaimInvite(ctx, store.ClaimInviteParams{CodeHash: token.Hash(inviteCode)})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInvite
		}
		if err != nil {
			return fmt.Errorf("claim invite: %w", err)
		}
		u, err = s.createUser(ctx, q, email, hash, RoleUser)
		if err != nil {
			return err // rolls back the claim: the invite stays usable
		}
		if err := q.SetInviteUsedBy(ctx, store.SetInviteUsedByParams{UsedBy: &u.ID, ID: inviteID}); err != nil {
			return fmt.Errorf("link invite: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	slog.InfoContext(ctx, "user registered", "user_id", u.ID)
	return u, nil
}

// CreateAdmin creates an admin account without an invite. Only the CLI calls
// it (kubectl exec), so it is not reachable over HTTP.
func (s *Service) CreateAdmin(ctx context.Context, email, pw string) (User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	hash, err := s.hasher.Hash(ctx, pw)
	if err != nil {
		if errors.Is(err, password.ErrLength) {
			return User{}, err
		}
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := s.createUser(ctx, s.q, email, hash, RoleAdmin)
	if err != nil {
		return User{}, err
	}
	slog.InfoContext(ctx, "admin created", "user_id", u.ID)
	return u, nil
}

func (s *Service) createUser(ctx context.Context, q *store.Queries, email, hash, role string) (User, error) {
	row, err := q.CreateUser(ctx, store.CreateUserParams{Email: email, PasswordHash: hash, Role: role})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}

// Login checks email and password and starts a new session. Every failure
// that depends on the account (unknown email, wrong password) returns
// ErrInvalidCredentials after the same amount of hashing work.
func (s *Service) Login(ctx context.Context, email, pw, userAgent string) (Session, error) {
	// No account can have a password over MaxLength, so don't spend an
	// argon2id run on it. This reveals nothing about which emails exist.
	if utf8.RuneCountInString(pw) > password.MaxLength {
		return Session{}, ErrInvalidCredentials
	}
	email, err := NormalizeEmail(email)
	if err != nil {
		return Session{}, s.failLogin(ctx, pw)
	}
	row, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, s.failLogin(ctx, pw)
	}
	if err != nil {
		return Session{}, fmt.Errorf("get user: %w", err)
	}
	ok, err := s.hasher.Verify(ctx, pw, row.PasswordHash)
	if err != nil {
		return Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		slog.InfoContext(ctx, "login failed", "user_id", row.ID, "reason", "wrong password")
		return Session{}, ErrInvalidCredentials
	}

	tok := token.New()
	expires := time.Now().Add(SessionTTL)
	err = s.q.CreateSession(ctx, store.CreateSessionParams{
		TokenHash: token.Hash(tok),
		UserID:    row.ID,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		UserAgent: cleanUserAgent(userAgent),
	})
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	slog.InfoContext(ctx, "user logged in", "user_id", row.ID)
	return Session{
		Token:     tok,
		ExpiresAt: expires,
		User:      User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time},
	}, nil
}

// failLogin burns the same hashing work as a real password check, then
// returns ErrInvalidCredentials.
func (s *Service) failLogin(ctx context.Context, pw string) error {
	dummy, err := s.dummyHash()
	if err != nil {
		return fmt.Errorf("dummy hash: %w", err)
	}
	if _, err := s.hasher.Verify(ctx, pw, dummy); err != nil {
		return fmt.Errorf("verify dummy: %w", err)
	}
	slog.InfoContext(ctx, "login failed", "reason", "unknown email")
	return ErrInvalidCredentials
}

// UserBySession returns the user behind a session token, or ErrNoSession
// when the token is empty, unknown or expired.
func (s *Service) UserBySession(ctx context.Context, tok string) (User, error) {
	if tok == "" {
		return User{}, ErrNoSession
	}
	row, err := s.q.GetSessionUser(ctx, token.Hash(tok))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNoSession
	}
	if err != nil {
		return User{}, fmt.Errorf("get session: %w", err)
	}
	return User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}

// Logout deletes the session behind tok. Unknown or empty tokens are not an
// error: logging out twice is fine.
func (s *Service) Logout(ctx context.Context, tok string) error {
	if tok == "" {
		return nil
	}
	u, err := s.UserBySession(ctx, tok)
	if errors.Is(err, ErrNoSession) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.q.DeleteSession(ctx, token.Hash(tok)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	slog.InfoContext(ctx, "user logged out", "user_id", u.ID)
	return nil
}

// DeleteExpiredSessions removes every expired session and returns how many.
func (s *Service) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	n, err := s.q.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return n, nil
}

// RunSessionCleanup deletes expired sessions now and then every interval,
// until ctx is cancelled. Run it in a goroutine. Several replicas may run it
// at once: deleting the same expired rows twice is harmless.
func (s *Service) RunSessionCleanup(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := s.DeleteExpiredSessions(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			slog.ErrorContext(ctx, "session cleanup failed", "err", err)
		case n > 0:
			slog.InfoContext(ctx, "expired sessions deleted", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// cleanUserAgent makes the header safe to store: valid UTF-8, at most
// maxUserAgent characters.
func cleanUserAgent(ua string) string {
	ua = strings.ToValidUTF8(ua, "")
	if utf8.RuneCountInString(ua) <= maxUserAgent {
		return ua
	}
	return string([]rune(ua)[:maxUserAgent])
}

// NormalizeEmail trims and lowercases an address and checks that it is a
// plain address (no display name like "Mo <mo@x.com>").
func NormalizeEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 254 {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || !strings.Contains(s[strings.LastIndexByte(s, '@')+1:], ".") {
		return "", ErrInvalidEmail
	}
	return s, nil
}
