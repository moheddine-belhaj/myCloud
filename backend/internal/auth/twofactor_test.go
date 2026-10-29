package auth

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/token"
	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/totp"
)

// clock fixes the service's clock at t and returns a function that moves it.
func clock(s *Service, t time.Time) func(time.Duration) {
	now := t
	s.now = func() time.Time { return now }
	return func(d time.Duration) { now = now.Add(d) }
}

// codeAt returns the code an authenticator app would show at the service's
// current time, from the base32 secret returned by SetupTOTP.
func codeAt(t *testing.T, s *Service, b32 string) string {
	t.Helper()
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(b32)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	return totp.Code(secret, totp.Step(s.now()))
}

// enable2FA sets up and enables 2FA for u and returns the base32 secret and
// the recovery codes.
func enable2FA(t *testing.T, s *Service, u User, sessionTok string) (string, []string) {
	t.Helper()
	ctx := context.Background()
	setup, err := s.SetupTOTP(ctx, u)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	codes, err := s.EnableTOTP(ctx, u, codeAt(t, s, setup.Secret), sessionTok)
	if err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}
	return setup.Secret, codes
}

var recoveryCodeFormat = regexp.MustCompile(`^[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}$`)

func TestSetupAndEnableTOTP(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)
	sess, err := s.Login(ctx, "mo@example.com", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	setup, err := s.SetupTOTP(ctx, u)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if len(setup.Secret) != 32 || !strings.HasPrefix(setup.URI, "otpauth://totp/MyCloud%20test:mo@example.com?") {
		t.Errorf("setup = %+v", setup)
	}

	// Stored encrypted: neither the raw nor the base32 secret is in the row.
	var enc []byte
	if err := pool.QueryRow(ctx, "SELECT secret_enc FROM user_totp WHERE user_id = $1", u.ID).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	raw, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if bytes.Contains(enc, raw) || bytes.Contains(enc, []byte(setup.Secret)) {
		t.Fatal("secret stored in plain text")
	}

	// Pending: not on yet, and a wrong code doesn't turn it on.
	if me, _ := s.UserBySession(ctx, sess.Token); me.TOTPEnabled {
		t.Error("TOTPEnabled before EnableTOTP")
	}
	if _, err := s.EnableTOTP(ctx, u, "000000", sess.Token); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("wrong code: err = %v, want ErrInvalidCode", err)
	}

	code := codeAt(t, s, setup.Secret)
	codes, err := s.EnableTOTP(ctx, u, code, sess.Token)
	if err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(codes), RecoveryCodeCount)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if !recoveryCodeFormat.MatchString(c) || seen[c] {
			t.Errorf("bad or duplicate recovery code %q", c)
		}
		seen[c] = true
	}
	// Only hashes are stored.
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM recovery_codes WHERE user_id = $1 AND code_hash = $2",
		u.ID, token.Hash(normalizeRecoveryCode(codes[0]))).Scan(&n); err != nil || n != 1 {
		t.Errorf("recovery code hash stored: n=%d err=%v", n, err)
	}

	if me, _ := s.UserBySession(ctx, sess.Token); !me.TOTPEnabled {
		t.Error("TOTPEnabled = false after EnableTOTP")
	}
	if _, err := s.SetupTOTP(ctx, u); !errors.Is(err, ErrTOTPAlreadyEnabled) {
		t.Errorf("setup when enabled: err = %v, want ErrTOTPAlreadyEnabled", err)
	}
	if _, err := s.EnableTOTP(ctx, u, code, sess.Token); !errors.Is(err, ErrTOTPAlreadyEnabled) {
		t.Errorf("enable twice: err = %v, want ErrTOTPAlreadyEnabled", err)
	}
}

func TestEnableTOTPWithoutSetup(t *testing.T) {
	s, _ := newTestService(t)
	u := newUser(t, s, "mo@example.com", RoleUser)
	if _, err := s.EnableTOTP(context.Background(), u, "123456", ""); !errors.Is(err, ErrTOTPNotSetUp) {
		t.Errorf("err = %v, want ErrTOTPNotSetUp", err)
	}
}

// Running setup again replaces the pending secret: only the newest one works.
func TestSetupReplacesPendingSecret(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)

	first, _ := s.SetupTOTP(ctx, u)
	second, _ := s.SetupTOTP(ctx, u)
	if first.Secret == second.Secret {
		t.Fatal("setup returned the same secret twice")
	}
	if codeAt(t, s, first.Secret) != codeAt(t, s, second.Secret) {
		if _, err := s.EnableTOTP(ctx, u, codeAt(t, s, first.Secret), ""); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("old secret: err = %v, want ErrInvalidCode", err)
		}
	}
	if _, err := s.EnableTOTP(ctx, u, codeAt(t, s, second.Secret), ""); err != nil {
		t.Errorf("new secret: %v", err)
	}
}

func TestEnableTOTPEndsOtherSessions(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)
	here, _ := s.Login(ctx, "mo@example.com", goodPassword, "laptop")
	there, _ := s.Login(ctx, "mo@example.com", goodPassword, "phone")

	enable2FA(t, s, u, here.Token)

	if _, err := s.UserBySession(ctx, here.Token); err != nil {
		t.Errorf("current session ended: %v", err)
	}
	if _, err := s.UserBySession(ctx, there.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("other session: err = %v, want ErrNoSession", err)
	}
}

func TestDisableTOTP(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	advance := clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)
	sess, _ := s.Login(ctx, "mo@example.com", goodPassword, "")
	secret, _ := enable2FA(t, s, u, sess.Token)

	// The code that enabled 2FA can't be used again.
	if err := s.DisableTOTP(ctx, u, goodPassword, codeAt(t, s, secret)); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("reused code: err = %v, want ErrInvalidCode", err)
	}
	advance(totp.Period)
	if err := s.DisableTOTP(ctx, u, "wrong password here", codeAt(t, s, secret)); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong password: err = %v, want ErrWrongPassword", err)
	}
	if err := s.DisableTOTP(ctx, u, goodPassword, "000000"); !errors.Is(err, ErrInvalidCode) && codeAt(t, s, secret) != "000000" {
		t.Errorf("wrong code: err = %v, want ErrInvalidCode", err)
	}
	if err := s.DisableTOTP(ctx, u, goodPassword, codeAt(t, s, secret)); err != nil {
		t.Fatalf("DisableTOTP: %v", err)
	}

	var rows int
	_ = pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM user_totp) + (SELECT count(*) FROM recovery_codes)").Scan(&rows)
	if rows != 0 {
		t.Errorf("%d TOTP / recovery code rows left, want 0", rows)
	}
	if me, _ := s.UserBySession(ctx, sess.Token); me.TOTPEnabled {
		t.Error("TOTPEnabled after DisableTOTP")
	}
	if err := s.DisableTOTP(ctx, u, goodPassword, "123456"); !errors.Is(err, ErrTOTPNotEnabled) {
		t.Errorf("disable twice: err = %v, want ErrTOTPNotEnabled", err)
	}
	// 2FA can be set up again afterwards.
	advance(totp.Period)
	enable2FA(t, s, u, sess.Token)
}

// A secret copied into another user's row doesn't decrypt: the user ID is
// part of the encryption.
func TestTOTPSecretBoundToUser(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	alice := newUser(t, s, "alice@example.com", RoleUser)
	bob := newUser(t, s, "bob@example.com", RoleUser)

	setupA, _ := s.SetupTOTP(ctx, alice)
	if _, err := s.SetupTOTP(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_totp SET secret_enc = (SELECT secret_enc FROM user_totp WHERE user_id = $1) WHERE user_id = $2`, alice.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	_, err := s.EnableTOTP(ctx, bob, codeAt(t, s, setupA.Secret), "")
	if err == nil || errors.Is(err, ErrInvalidCode) {
		t.Errorf("copied secret: err = %v, want a decryption error", err)
	}
}

func TestTOTPUnavailableWithoutKey(t *testing.T) {
	s, _ := newTestService(t)
	s.totpKeys = nil // as in the CLI
	u := newUser(t, s, "mo@example.com", RoleUser)
	if _, err := s.SetupTOTP(context.Background(), u); !errors.Is(err, ErrTOTPUnavailable) {
		t.Errorf("err = %v, want ErrTOTPUnavailable", err)
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	for _, in := range []string{"k7qx-m2pd-4hzt-wn6a", " K7QX M2PD-4hzt-WN6A\n", "k7qxm2pd4hztwn6a"} {
		if got := normalizeRecoveryCode(in); got != "k7qxm2pd4hztwn6a" {
			t.Errorf("normalizeRecoveryCode(%q) = %q", in, got)
		}
	}
}

// The whole flow over HTTP, with the session cookie.
func TestTOTPHTTP(t *testing.T) {
	h, s := newTestAPI(t)
	advance := clock(s, time.Unix(1_800_000_000, 0))
	newUser(t, s, "mo@example.com", RoleUser)
	c := login(t, h, "mo@example.com")

	if rec := do(h, http.MethodPost, "/api/auth/2fa/setup", `{}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("setup without session = %d, want 401", rec.Code)
	}
	rec := do(h, http.MethodPost, "/api/auth/2fa/setup", `{}`, c.Value)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("setup = %d %s (Cache-Control %q)", rec.Code, rec.Body, rec.Header().Get("Cache-Control"))
	}
	var setup TOTPSetup
	_ = json.NewDecoder(rec.Body).Decode(&setup)

	if rec := do(h, http.MethodPost, "/api/auth/2fa/enable", `{"code":"000000"}`, c.Value); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_code" {
		if codeAt(t, s, setup.Secret) != "000000" {
			t.Errorf("wrong code = %d %s, want 400 invalid_code", rec.Code, rec.Body)
		}
	}
	rec = do(h, http.MethodPost, "/api/auth/2fa/enable", `{"code":"`+codeAt(t, s, setup.Secret)+`"}`, c.Value)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	var en totpEnableResponse
	if err := json.NewDecoder(rec.Body).Decode(&en); err != nil || len(en.RecoveryCodes) != RecoveryCodeCount {
		t.Fatalf("enable body = %+v, err %v", en, err)
	}

	rec = do(h, http.MethodGet, "/api/auth/me", "", c.Value)
	if !strings.Contains(rec.Body.String(), `"totpEnabled":true`) {
		t.Errorf("me = %s, want totpEnabled true", rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/auth/2fa/setup", `{}`, c.Value); rec.Code != http.StatusConflict || errorCode(t, rec) != "totp_already_enabled" {
		t.Errorf("setup again = %d %s, want 409 totp_already_enabled", rec.Code, rec.Body)
	}

	advance(totp.Period)
	code := codeAt(t, s, setup.Secret)
	if rec := do(h, http.MethodPost, "/api/auth/2fa/disable", `{"password":"wrong password here","code":"`+code+`"}`, c.Value); rec.Code != http.StatusForbidden || errorCode(t, rec) != "wrong_password" {
		t.Errorf("disable wrong password = %d %s, want 403 wrong_password", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/auth/2fa/disable", `{"password":"`+goodPassword+`","code":"`+code+`"}`, c.Value); rec.Code != http.StatusNoContent {
		t.Fatalf("disable = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/auth/2fa/disable", `{"password":"`+goodPassword+`","code":"`+code+`"}`, c.Value); rec.Code != http.StatusConflict || errorCode(t, rec) != "totp_not_enabled" {
		t.Errorf("disable again = %d %s, want 409 totp_not_enabled", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/auth/2fa/enable", `{"code":"123456"}`, c.Value); rec.Code != http.StatusConflict || errorCode(t, rec) != "totp_not_set_up" {
		t.Errorf("enable without setup = %d %s, want 409 totp_not_set_up", rec.Code, rec.Body)
	}
}
