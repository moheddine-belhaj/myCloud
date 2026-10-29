package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/keyring"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/password"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/store"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/token"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/totp"
)

// DefaultTOTPIssuer is the account title in authenticator apps when
// TOTP_ISSUER is not set.
const DefaultTOTPIssuer = "MyCloud"

// RecoveryCodeCount is how many recovery codes a user gets when 2FA is enabled.
const RecoveryCodeCount = 10

// 2FA errors. The HTTP layer maps each to a status code.
var (
	ErrTOTPUnavailable    = errors.New("two-factor authentication is not configured on this server")
	ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already enabled")
	ErrTOTPNotSetUp       = errors.New("start two-factor setup first")
	ErrTOTPNotEnabled     = errors.New("two-factor authentication is not enabled")
	// ErrInvalidCode covers wrong, expired and already used codes alike.
	ErrInvalidCode   = errors.New("invalid authentication code")
	ErrWrongPassword = errors.New("wrong password")
)

// TOTPSetup is what a user needs to add MyCloud to an authenticator app.
// It is shown once; the server only keeps the encrypted secret.
type TOTPSetup struct {
	Secret string `json:"secret"` // base32, for typing in by hand
	URI    string `json:"uri"`    // otpauth:// URI, for a QR code
}

// WithTOTP turns on two-factor authentication. keys encrypts the TOTP
// secrets; issuer is the title shown in authenticator apps ("MyCloud dev").
// Without it, the 2FA methods return ErrTOTPUnavailable (the CLI doesn't need them).
func (s *Service) WithTOTP(keys *keyring.Keyring, issuer string) *Service {
	if issuer == "" {
		issuer = DefaultTOTPIssuer
	}
	s.totpKeys, s.totpIssuer = keys, issuer
	return s
}

// SetupTOTP creates a new secret for u and stores it encrypted, not yet
// active. Calling it again before EnableTOTP replaces the pending secret.
func (s *Service) SetupTOTP(ctx context.Context, u User) (TOTPSetup, error) {
	if s.totpKeys == nil {
		return TOTPSetup{}, ErrTOTPUnavailable
	}
	secret := totp.NewSecret()
	// The user ID is the associated data: this ciphertext only opens for u.
	enc, err := s.totpKeys.Seal(secret, u.ID[:])
	if err != nil {
		return TOTPSetup{}, fmt.Errorf("seal totp secret: %w", err)
	}
	n, err := s.q.StartTOTPSetup(ctx, store.StartTOTPSetupParams{UserID: u.ID, SecretEnc: enc})
	if err != nil {
		return TOTPSetup{}, fmt.Errorf("store totp secret: %w", err)
	}
	if n == 0 {
		return TOTPSetup{}, ErrTOTPAlreadyEnabled
	}
	slog.InfoContext(ctx, "2fa setup started", "user_id", u.ID)
	return TOTPSetup{Secret: totp.EncodeSecret(secret), URI: totp.URI(s.totpIssuer, u.Email, secret)}, nil
}

// EnableTOTP activates the pending secret once the user proves their app
// works with a valid code. It returns the recovery codes (shown once) and
// ends every other session of the user; sessionTok, the current one, stays.
func (s *Service) EnableTOTP(ctx context.Context, u User, code, sessionTok string) ([]string, error) {
	if s.totpKeys == nil {
		return nil, ErrTOTPUnavailable
	}
	row, err := s.q.GetTOTP(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTOTPNotSetUp
	}
	if err != nil {
		return nil, fmt.Errorf("get totp: %w", err)
	}
	if row.EnabledAt.Valid {
		return nil, ErrTOTPAlreadyEnabled
	}
	step, err := s.checkCode(ctx, u, row, code)
	if err != nil {
		return nil, err
	}

	codes, hashes := newRecoveryCodes()
	var ended int64
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.EnableTOTP(ctx, store.EnableTOTPParams{UserID: u.ID, Step: step})
		if err != nil {
			return fmt.Errorf("enable totp: %w", err)
		}
		if n == 0 { // enabled by a parallel request, or that step was just used
			return ErrInvalidCode
		}
		if err := q.DeleteRecoveryCodes(ctx, u.ID); err != nil {
			return fmt.Errorf("delete recovery codes: %w", err)
		}
		if err := q.InsertRecoveryCodes(ctx, store.InsertRecoveryCodesParams{UserID: u.ID, CodeHashes: hashes}); err != nil {
			return fmt.Errorf("insert recovery codes: %w", err)
		}
		// A password thief may already be logged in elsewhere: end those sessions.
		ended, err = q.DeleteOtherSessions(ctx, store.DeleteOtherSessionsParams{UserID: u.ID, KeepTokenHash: token.Hash(sessionTok)})
		if err != nil {
			return fmt.Errorf("end other sessions: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "2fa enabled", "user_id", u.ID, "other_sessions_ended", ended)
	return codes, nil
}

// DisableTOTP turns 2FA off. It needs the password and a current code (or a
// recovery code, for a lost phone), so a stolen session alone can't remove
// the second factor.
func (s *Service) DisableTOTP(ctx context.Context, u User, pw, code string) error {
	if s.totpKeys == nil {
		return ErrTOTPUnavailable
	}
	row, err := s.q.GetTOTP(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTOTPNotEnabled
	}
	if err != nil {
		return fmt.Errorf("get totp: %w", err)
	}
	if !row.EnabledAt.Valid {
		return ErrTOTPNotEnabled
	}
	if err := s.checkPassword(ctx, u, pw); err != nil {
		return err
	}
	if _, err := s.checkSecondFactor(ctx, u.ID, code, true); err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteTOTP(ctx, u.ID); err != nil {
			return fmt.Errorf("delete totp: %w", err)
		}
		if err := q.DeleteRecoveryCodes(ctx, u.ID); err != nil {
			return fmt.Errorf("delete recovery codes: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "2fa disabled", "user_id", u.ID)
	return nil
}

// checkCode decrypts the user's secret and validates code against it. It
// returns the matched time step, which the caller must record.
func (s *Service) checkCode(ctx context.Context, u User, row store.GetTOTPRow, code string) (int64, error) {
	secret, err := s.totpKeys.Open(row.SecretEnc, u.ID[:])
	if err != nil {
		// Wrong key in this environment or a tampered row: a server problem,
		// not a user mistake.
		return 0, fmt.Errorf("open totp secret: %w", err)
	}
	step, ok := totp.Validate(secret, code, s.now(), row.LastStep)
	if !ok {
		slog.InfoContext(ctx, "2fa code rejected", "user_id", u.ID)
		return 0, ErrInvalidCode
	}
	return step, nil
}

// checkPassword verifies the password of a logged-in user.
func (s *Service) checkPassword(ctx context.Context, u User, pw string) error {
	if utf8.RuneCountInString(pw) > password.MaxLength {
		return ErrWrongPassword
	}
	hash, err := s.q.GetPasswordHash(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("get password hash: %w", err)
	}
	ok, err := s.hasher.Verify(ctx, pw, hash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		slog.InfoContext(ctx, "password check failed", "user_id", u.ID)
		return ErrWrongPassword
	}
	return nil
}

// newRecoveryCodes returns RecoveryCodeCount codes like "k7qx-m2pd-4hzt-wn6a"
// (80 random bits each) and their SHA-256 hashes for storage.
func newRecoveryCodes() (codes []string, hashes [][]byte) {
	for range RecoveryCodeCount {
		raw := strings.ToLower(rand.Text()[:16]) // base32: no 0/1/8/9, nothing to confuse
		code := raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16]
		codes = append(codes, code)
		hashes = append(hashes, token.Hash(normalizeRecoveryCode(code)))
	}
	return codes, hashes
}

// normalizeRecoveryCode makes "K7QX M2PD-4hzt-wn6a" and "k7qxm2pd4hztwn6a" equal.
func normalizeRecoveryCode(c string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(c)))
}
