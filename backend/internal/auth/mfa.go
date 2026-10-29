package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/store"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/token"
)

// The second login step (M3 PR 3).
const (
	// MFAChallengeTTL is how long the user has to enter the code after the password.
	MFAChallengeTTL = 5 * time.Minute
	// MaxMFAAttempts is how many codes one challenge accepts before it dies.
	MaxMFAAttempts = 5
)

// ErrNoChallenge means the 2FA challenge is missing, unknown, expired or out
// of attempts: the user has to enter the password again.
var ErrNoChallenge = errors.New("2fa challenge is missing, expired or used up")

// Challenge is a half-finished login. Token goes into the __Host-mfa cookie
// and is never stored; the database keeps its SHA-256.
type Challenge struct {
	Token     string
	ExpiresAt time.Time
}

// MFARequiredError is what Login returns when the password is right and the
// account has 2FA on. It is not a failure: the caller sends the challenge
// token to the client and finishes with LoginTOTP.
type MFARequiredError struct {
	Challenge Challenge
}

func (e *MFARequiredError) Error() string { return "two-factor code required" }

// recoveryCodeRe matches a normalized recovery code: 16 base32 characters.
var recoveryCodeRe = regexp.MustCompile(`^[a-z2-7]{16}$`)

// totpEnabled reports whether the user has finished 2FA setup.
func (s *Service) totpEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	row, err := s.q.GetTOTP(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get totp: %w", err)
	}
	return row.EnabledAt.Valid, nil
}

func (s *Service) startChallenge(ctx context.Context, userID uuid.UUID, userAgent string) (Challenge, error) {
	tok := token.New()
	expires := time.Now().Add(MFAChallengeTTL)
	err := s.q.CreateMFAChallenge(ctx, store.CreateMFAChallengeParams{
		TokenHash: token.Hash(tok),
		UserID:    userID,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		UserAgent: cleanUserAgent(userAgent),
	})
	if err != nil {
		return Challenge{}, fmt.Errorf("create 2fa challenge: %w", err)
	}
	return Challenge{Token: tok, ExpiresAt: expires}, nil
}

// LoginTOTP finishes a login started by Login: it checks a TOTP code or a
// recovery code against the challenge and starts the session.
//
// Each call counts as one attempt, whether the code is right or not. After
// MaxMFAAttempts wrong codes the challenge is deleted and the next call gets
// ErrNoChallenge.
func (s *Service) LoginTOTP(ctx context.Context, challengeTok, code, userAgent string) (Session, error) {
	if s.totpKeys == nil {
		return Session{}, ErrTOTPUnavailable
	}
	if challengeTok == "" {
		return Session{}, ErrNoChallenge
	}
	hash := token.Hash(challengeTok)
	ch, err := s.q.CountMFAAttempt(ctx, store.CountMFAAttemptParams{TokenHash: hash, MaxAttempts: MaxMFAAttempts})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoChallenge
	}
	if err != nil {
		return Session{}, fmt.Errorf("count 2fa attempt: %w", err)
	}

	method, err := s.checkSecondFactor(ctx, ch.UserID, code, true)
	if errors.Is(err, ErrInvalidCode) {
		if ch.Attempts >= MaxMFAAttempts {
			if err := s.q.DeleteMFAChallenge(ctx, hash); err != nil {
				return Session{}, fmt.Errorf("delete 2fa challenge: %w", err)
			}
			slog.WarnContext(ctx, "2fa challenge used up", "user_id", ch.UserID, "attempts", ch.Attempts)
		}
		return Session{}, ErrInvalidCode
	}
	if err != nil {
		return Session{}, err
	}

	row, err := s.q.GetUserByID(ctx, ch.UserID)
	if err != nil {
		return Session{}, fmt.Errorf("get user: %w", err)
	}
	u := User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time, TOTPEnabled: true}
	var sess Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		// One challenge, one session: delete it in the same transaction.
		if err := q.DeleteMFAChallenge(ctx, hash); err != nil {
			return fmt.Errorf("delete 2fa challenge: %w", err)
		}
		sess, err = s.newSession(ctx, q, u, userAgent)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	slog.InfoContext(ctx, "user logged in", "user_id", u.ID, "second_factor", method)
	return sess, nil
}

// checkSecondFactor checks a TOTP code, or a recovery code when allowRecovery
// is set, and uses it up. It returns "totp" or "recovery_code" for the log.
func (s *Service) checkSecondFactor(ctx context.Context, userID uuid.UUID, code string, allowRecovery bool) (string, error) {
	if rc := normalizeRecoveryCode(code); recoveryCodeRe.MatchString(rc) {
		if !allowRecovery {
			return "", ErrInvalidCode
		}
		n, err := s.q.UseRecoveryCode(ctx, store.UseRecoveryCodeParams{UserID: userID, CodeHash: token.Hash(rc)})
		if err != nil {
			return "", fmt.Errorf("use recovery code: %w", err)
		}
		if n == 0 {
			slog.InfoContext(ctx, "recovery code rejected", "user_id", userID)
			return "", ErrInvalidCode
		}
		left, err := s.q.CountUnusedRecoveryCodes(ctx, userID)
		if err != nil {
			return "", fmt.Errorf("count recovery codes: %w", err)
		}
		slog.WarnContext(ctx, "recovery code used", "user_id", userID, "recovery_codes_left", left)
		return "recovery_code", nil
	}

	row, err := s.q.GetTOTP(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidCode // 2FA was turned off meanwhile
	}
	if err != nil {
		return "", fmt.Errorf("get totp: %w", err)
	}
	if !row.EnabledAt.Valid {
		return "", ErrInvalidCode
	}
	step, err := s.checkCode(ctx, User{ID: userID}, row, code)
	if err != nil {
		return "", err
	}
	// Record the step atomically: a code used twice in parallel wins once.
	n, err := s.q.UseTOTPStep(ctx, store.UseTOTPStepParams{UserID: userID, Step: step})
	if err != nil {
		return "", fmt.Errorf("use totp step: %w", err)
	}
	if n == 0 {
		return "", ErrInvalidCode
	}
	return "totp", nil
}

// RegenerateRecoveryCodes replaces all recovery codes of u. It needs a code
// from the authenticator app; a recovery code is not accepted here.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, u User, code string) ([]string, error) {
	if s.totpKeys == nil {
		return nil, ErrTOTPUnavailable
	}
	on, err := s.totpEnabled(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if !on {
		return nil, ErrTOTPNotEnabled
	}
	if _, err := s.checkSecondFactor(ctx, u.ID, code, false); err != nil {
		return nil, err
	}
	codes, hashes := newRecoveryCodes()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteRecoveryCodes(ctx, u.ID); err != nil {
			return fmt.Errorf("delete recovery codes: %w", err)
		}
		if err := q.InsertRecoveryCodes(ctx, store.InsertRecoveryCodesParams{UserID: u.ID, CodeHashes: hashes}); err != nil {
			return fmt.Errorf("insert recovery codes: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "recovery codes regenerated", "user_id", u.ID)
	return codes, nil
}

// ResetTOTP removes 2FA from an account and ends all its sessions. Only the
// CLI calls it (kubectl exec), for a user who lost both phone and recovery
// codes. The user logs in with the password and sets up 2FA again.
func (s *Service) ResetTOTP(ctx context.Context, email string) (User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	row, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	var ended int64
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteTOTP(ctx, row.ID); err != nil {
			return fmt.Errorf("delete totp: %w", err)
		}
		if err := q.DeleteRecoveryCodes(ctx, row.ID); err != nil {
			return fmt.Errorf("delete recovery codes: %w", err)
		}
		ended, err = q.DeleteUserSessions(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("end sessions: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	slog.WarnContext(ctx, "2fa reset by admin (cli)", "user_id", row.ID, "sessions_ended", ended)
	return User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}
