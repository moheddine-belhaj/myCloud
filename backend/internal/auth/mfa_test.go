package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/totp"
)

// totpSecrets remembers the base32 secret per service and email, so the
// login helper can answer the 2FA step.
var totpSecrets sync.Map

// enableTOTPFor turns 2FA on for u and remembers its secret. It returns the
// secret and the recovery codes.
func enableTOTPFor(t *testing.T, s *Service, u User) (string, []string) {
	t.Helper()
	nextStep(s)
	secret, codes := enable2FA(t, s, u, "")
	totpSecrets.Store(s2k(s, u.Email), secret)
	return secret, codes
}

type secretKey struct {
	s     *Service
	email string
}

func s2k(s *Service, email string) secretKey { return secretKey{s, strings.ToLower(email)} }

// nextStep moves the service clock 30 s forward, so the next code is new
// (each code works only once).
func nextStep(s *Service) {
	prev := s.now
	s.now = func() time.Time { return prev().Add(totp.Period) }
}

// nextCode returns a fresh, unused code for secret.
func nextCode(t *testing.T, s *Service, secret string) string {
	t.Helper()
	nextStep(s)
	return codeAt(t, s, secret)
}

// finishTOTPLogin answers the 2FA step of an HTTP login with the remembered secret.
func finishTOTPLogin(t *testing.T, h http.Handler, email string, rec *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	mfa := cookie(rec, MFACookieName)
	if mfa == nil || !strings.Contains(rec.Body.String(), `"mfaRequired":true`) {
		t.Fatalf("login %s = 200 %s without an mfa cookie", email, rec.Body)
	}
	api := h.(*testAPI)
	v, ok := totpSecrets.Load(s2k(api.s, email))
	if !ok {
		t.Fatalf("no TOTP secret remembered for %s", email)
	}
	return doMFA(h, `{"code":"`+nextCode(t, api.s, v.(string))+`"}`, mfa.Value)
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// doMFA posts to the second login step with a __Host-mfa cookie.
func doMFA(h http.Handler, body, mfaTok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login/totp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mfaTok != "" {
		req.AddCookie(&http.Cookie{Name: MFACookieName, Value: mfaTok})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// startLogin logs in with the password and returns the 2FA challenge.
func startLogin(t *testing.T, s *Service, email string) Challenge {
	t.Helper()
	_, err := s.Login(context.Background(), email, goodPassword, "test")
	var mfa *MFARequiredError
	if !errors.As(err, &mfa) {
		t.Fatalf("Login(%s): err = %v, want MFARequiredError", email, err)
	}
	return mfa.Challenge
}

func TestLoginWithTOTP(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)
	secret, _ := enableTOTPFor(t, s, u)

	ch := startLogin(t, s, "mo@example.com")
	if len(ch.Token) != 43 || time.Until(ch.ExpiresAt) > MFAChallengeTTL {
		t.Errorf("challenge = %+v", ch)
	}
	if _, err := s.LoginTOTP(ctx, ch.Token, "000000", ""); !errors.Is(err, ErrInvalidCode) && codeAt(t, s, secret) != "000000" {
		t.Errorf("wrong code: err = %v, want ErrInvalidCode", err)
	}
	good := nextCode(t, s, secret)
	sess, err := s.LoginTOTP(ctx, ch.Token, good, "test")
	if err != nil {
		t.Fatalf("LoginTOTP: %v", err)
	}
	if me, err := s.UserBySession(ctx, sess.Token); err != nil || me.ID != u.ID || !me.TOTPEnabled {
		t.Errorf("session user = %+v, %v", me, err)
	}
	// The challenge is gone: it can't make a second session.
	if _, err := s.LoginTOTP(ctx, ch.Token, good, ""); !errors.Is(err, ErrNoChallenge) {
		t.Errorf("reused challenge: err = %v, want ErrNoChallenge", err)
	}
	// A code used for one login can't be used for the next one.
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, good, ""); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replayed code: err = %v, want ErrInvalidCode", err)
	}
}

// A wrong password never reaches the 2FA step.
func TestLoginWithTOTPWrongPassword(t *testing.T) {
	s, _ := newTestService(t)
	enableTOTPFor(t, s, newUser(t, s, "mo@example.com", RoleUser))
	if _, err := s.Login(context.Background(), "mo@example.com", "wrong password here", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginTOTPAttemptLimit(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	secret, _ := enableTOTPFor(t, s, newUser(t, s, "mo@example.com", RoleUser))
	ch := startLogin(t, s, "mo@example.com")

	wrong := "000000"
	for i := 1; i <= MaxMFAAttempts; i++ {
		if codeAt(t, s, secret) == wrong {
			wrong = "999999"
		}
		if _, err := s.LoginTOTP(ctx, ch.Token, wrong, ""); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("attempt %d: err = %v, want ErrInvalidCode", i, err)
		}
	}
	// The 6th attempt is refused even with the right code.
	if _, err := s.LoginTOTP(ctx, ch.Token, nextCode(t, s, secret), ""); !errors.Is(err, ErrNoChallenge) {
		t.Errorf("attempt %d: err = %v, want ErrNoChallenge", MaxMFAAttempts+1, err)
	}
	// A new password login gets a new challenge.
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, nextCode(t, s, secret), ""); err != nil {
		t.Errorf("new challenge: %v", err)
	}
}

func TestLoginTOTPExpiredChallenge(t *testing.T) {
	s, pool := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	secret, _ := enableTOTPFor(t, s, newUser(t, s, "mo@example.com", RoleUser))
	ch := startLogin(t, s, "mo@example.com")
	if _, err := pool.Exec(ctx, "UPDATE mfa_challenges SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoginTOTP(ctx, ch.Token, nextCode(t, s, secret), ""); !errors.Is(err, ErrNoChallenge) {
		t.Errorf("err = %v, want ErrNoChallenge", err)
	}
	if n, err := s.q.DeleteExpiredMFAChallenges(ctx); err != nil || n != 1 {
		t.Errorf("DeleteExpiredMFAChallenges = %d, %v; want 1", n, err)
	}
	if _, err := s.LoginTOTP(ctx, "", "123456", ""); !errors.Is(err, ErrNoChallenge) {
		t.Errorf("empty token: err = %v, want ErrNoChallenge", err)
	}
}

func TestLoginWithRecoveryCode(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)
	_, codes := enableTOTPFor(t, s, u)

	// Typed sloppily: upper case, spaces instead of dashes.
	typed := strings.ToUpper(strings.ReplaceAll(codes[0], "-", " "))
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, typed, ""); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, codes[0], ""); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("same recovery code again: err = %v, want ErrInvalidCode", err)
	}
	if left, _ := s.q.CountUnusedRecoveryCodes(ctx, u.ID); left != RecoveryCodeCount-1 {
		t.Errorf("recovery codes left = %d, want %d", left, RecoveryCodeCount-1)
	}
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, "aaaa-bbbb-cccc-dddd", ""); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("unknown recovery code: err = %v, want ErrInvalidCode", err)
	}
}

func TestDisableTOTPWithRecoveryCode(t *testing.T) {
	s, _ := newTestService(t)
	u := newUser(t, s, "mo@example.com", RoleUser)
	_, codes := enableTOTPFor(t, s, u)
	if err := s.DisableTOTP(context.Background(), u, goodPassword, codes[3]); err != nil {
		t.Fatalf("DisableTOTP with recovery code: %v", err)
	}
}

func TestRegenerateRecoveryCodes(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	u := newUser(t, s, "mo@example.com", RoleUser)
	secret, old := enableTOTPFor(t, s, u)

	if _, err := s.RegenerateRecoveryCodes(ctx, u, old[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("with a recovery code: err = %v, want ErrInvalidCode", err)
	}
	fresh, err := s.RegenerateRecoveryCodes(ctx, u, nextCode(t, s, secret))
	if err != nil || len(fresh) != RecoveryCodeCount {
		t.Fatalf("RegenerateRecoveryCodes = %d codes, %v", len(fresh), err)
	}
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, old[1], ""); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("old code after regenerate: err = %v, want ErrInvalidCode", err)
	}
	if _, err := s.LoginTOTP(ctx, startLogin(t, s, "mo@example.com").Token, fresh[1], ""); err != nil {
		t.Errorf("new code: %v", err)
	}
}

func TestResetTOTP(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	u := newUser(t, s, "mo@example.com", RoleUser)
	sess, _ := s.Login(ctx, "mo@example.com", goodPassword, "")
	enableTOTPFor(t, s, u)

	got, err := s.ResetTOTP(ctx, " MO@example.com ")
	if err != nil || got.ID != u.ID {
		t.Fatalf("ResetTOTP = %+v, %v", got, err)
	}
	if _, err := s.UserBySession(ctx, sess.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("old session: err = %v, want ErrNoSession", err)
	}
	// Password alone works again.
	if _, err := s.Login(ctx, "mo@example.com", goodPassword, ""); err != nil {
		t.Errorf("Login after reset: %v", err)
	}
	if _, err := s.ResetTOTP(ctx, "nobody@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown email: err = %v, want ErrUserNotFound", err)
	}
}

func TestLoginTOTPHTTP(t *testing.T) {
	h, s := newTestAPI(t)
	u := newUser(t, s, "mo@example.com", RoleUser)
	secret, codes := enableTOTPFor(t, s, u)

	rec := do(h, http.MethodPost, "/api/auth/login", `{"email":"mo@example.com","password":"`+goodPassword+`"}`, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mfaRequired":true`) {
		t.Fatalf("login = %d %s, want 200 mfaRequired", rec.Code, rec.Body)
	}
	if cookie(rec, CookieName) != nil {
		t.Fatal("password step set a session cookie")
	}
	mfa := cookie(rec, MFACookieName)
	if mfa == nil || !mfa.HttpOnly || !mfa.Secure || mfa.SameSite != http.SameSiteStrictMode || mfa.Path != "/" ||
		mfa.MaxAge > int(MFAChallengeTTL.Seconds()) || mfa.MaxAge < int(MFAChallengeTTL.Seconds())-60 {
		t.Fatalf("mfa cookie = %+v", mfa)
	}

	if rec := doMFA(h, `{"code":"123456"}`, ""); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "mfa_expired" {
		t.Errorf("no mfa cookie = %d %s, want 401 mfa_expired", rec.Code, rec.Body)
	}
	wrong := "000000"
	if codeAt(t, s, secret) == wrong {
		wrong = "999999"
	}
	if rec := doMFA(h, `{"code":"`+wrong+`"}`, mfa.Value); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_code" {
		t.Errorf("wrong code = %d %s, want 400 invalid_code", rec.Code, rec.Body)
	}
	rec = doMFA(h, `{"code":"`+nextCode(t, s, secret)+`"}`, mfa.Value)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second step = %d %s, want 204", rec.Code, rec.Body)
	}
	sess := cookie(rec, CookieName)
	if sess == nil {
		t.Fatal("no session cookie after the code")
	}
	if c := cookie(rec, MFACookieName); c == nil || c.MaxAge >= 0 {
		t.Errorf("mfa cookie not cleared: %+v", c)
	}
	if rec := do(h, http.MethodGet, "/api/auth/me", "", sess.Value); rec.Code != http.StatusOK {
		t.Errorf("me = %d", rec.Code)
	}

	// New recovery codes over HTTP.
	rec = do(h, http.MethodPost, "/api/auth/2fa/recovery-codes", `{"code":"`+codes[0]+`"}`, sess.Value)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_code" {
		t.Errorf("recovery-codes with a recovery code = %d %s, want 400", rec.Code, rec.Body)
	}
	rec = do(h, http.MethodPost, "/api/auth/2fa/recovery-codes", `{"code":"`+nextCode(t, s, secret)+`"}`, sess.Value)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "recoveryCodes") {
		t.Errorf("recovery-codes = %d %s, want 200", rec.Code, rec.Body)
	}
}

// Admin features need 2FA; the admin can still log in and turn it on.
func TestAdminNeeds2FA(t *testing.T) {
	h, s := newTestAPI(t)
	admin := newUser(t, s, "admin@example.com", RoleAdmin)
	c := login(t, h, "admin@example.com")

	rec := do(h, http.MethodPost, "/api/auth/invites", `{}`, c.Value)
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "totp_required" {
		t.Fatalf("invite without 2FA = %d %s, want 403 totp_required", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/auth/2fa/setup", `{}`, c.Value); rec.Code != http.StatusOK {
		t.Errorf("admin can start 2FA setup: %d %s", rec.Code, rec.Body)
	}
	enableTOTPFor(t, s, admin) // ends the old session too
	c = login(t, h, "admin@example.com")
	if rec := do(h, http.MethodPost, "/api/auth/invites", `{}`, c.Value); rec.Code != http.StatusCreated {
		t.Errorf("invite with 2FA = %d %s, want 201", rec.Code, rec.Body)
	}
}

// Parallel guesses can't get more than MaxMFAAttempts tries out of one challenge.
func TestLoginTOTPAttemptLimitParallel(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	clock(s, time.Unix(1_800_000_000, 0))
	secret, _ := enableTOTPFor(t, s, newUser(t, s, "mo@example.com", RoleUser))
	ch := startLogin(t, s, "mo@example.com")
	wrong := "000000"
	if codeAt(t, s, secret) == wrong {
		wrong = "999999"
	}

	const tries = 3 * MaxMFAAttempts
	errs := make(chan error, tries)
	var wg sync.WaitGroup
	for range tries {
		wg.Go(func() {
			_, err := s.LoginTOTP(ctx, ch.Token, wrong, "")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	counted := 0
	for err := range errs {
		switch {
		case errors.Is(err, ErrInvalidCode):
			counted++
		case !errors.Is(err, ErrNoChallenge):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if counted != MaxMFAAttempts {
		t.Errorf("%d codes were checked, want exactly %d", counted, MaxMFAAttempts)
	}
}
