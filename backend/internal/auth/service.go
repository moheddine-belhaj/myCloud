// Package auth is the business logic of the auth service: invites,
// registration and (from PR 3) login and sessions.
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
	"time"

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

// Errors returned to callers. The HTTP layer maps each to a status code.
var (
	ErrInvalidEmail  = errors.New("invalid email address")
	ErrEmailTaken    = errors.New("email is already registered")
	ErrInvalidInvite = errors.New("invite code is invalid, used or expired")
	ErrInvalidTTL    = fmt.Errorf("invite lifetime must be between 1h and %s", MaxInviteTTL)
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

// Service implements the auth use cases on top of the auth database.
type Service struct {
	pool   *pgxpool.Pool
	q      *store.Queries
	hasher *password.Hasher
}

// NewService returns a Service using pool for storage and hasher for passwords.
func NewService(pool *pgxpool.Pool, hasher *password.Hasher) *Service {
	return &Service{pool: pool, q: store.New(pool), hasher: hasher}
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
	slog.InfoContext(ctx, "invite created", "invite_id", row.ID, "expires_at", row.ExpiresAt.Time)
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
