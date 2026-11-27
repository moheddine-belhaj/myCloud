package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newTestLimiter connects to the throwaway Redis in TEST_REDIS_ADDR and empties it.
func newTestLimiter(t *testing.T) (*Limiter, *redis.Client) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush test redis: %v", err)
	}
	return NewLimiter(rdb), rdb
}

func mustLocked(t *testing.T, l *Limiter, email, ip, wantRule string) {
	t.Helper()
	wait, rule, err := l.Locked(context.Background(), email, ip)
	if err != nil {
		t.Fatalf("Locked: %v", err)
	}
	if wantRule == "" {
		if wait > 0 {
			t.Fatalf("locked by %s for %s, want not locked", rule, wait)
		}
		return
	}
	if wait <= 0 || rule != wantRule {
		t.Fatalf("Locked = %s by %q, want locked by %q", wait, rule, wantRule)
	}
}

func fail(t *testing.T, l *Limiter, email, ip string, n int) {
	t.Helper()
	for range n {
		if err := l.Fail(context.Background(), email, ip); err != nil {
			t.Fatalf("Fail: %v", err)
		}
	}
}

func TestLimiterAccountIP(t *testing.T) {
	l, _ := newTestLimiter(t)
	fail(t, l, "mo@example.com", "203.0.113.7", AccountIPLimit-1)
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "")

	fail(t, l, "mo@example.com", "203.0.113.7", 1)
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "account_ip")
	mustLocked(t, l, "mo@example.com", "198.51.100.9", "")
	mustLocked(t, l, "other@example.com", "203.0.113.7", "")

	wait, _, _ := l.Locked(context.Background(), "mo@example.com", "203.0.113.7")
	if wait > AccountIPWindow || wait < AccountIPWindow-time.Minute {
		t.Errorf("lock lasts %s, want about %s", wait, AccountIPWindow)
	}

	if err := l.Reset(context.Background(), "mo@example.com", "203.0.113.7"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "")
}

func TestLimiterIP(t *testing.T) {
	l, _ := newTestLimiter(t)
	for i := range IPLimit {
		fail(t, l, fmt.Sprintf("user%d@example.com", i), "203.0.113.7", 1)
	}
	mustLocked(t, l, "new@example.com", "203.0.113.7", "ip")
	mustLocked(t, l, "", "203.0.113.7", "ip")
	mustLocked(t, l, "new@example.com", "198.51.100.9", "")
}

func TestLimiterAccount(t *testing.T) {
	l, _ := newTestLimiter(t)
	for i := range AccountLimit {
		fail(t, l, "mo@example.com", fmt.Sprintf("198.51.100.%d", i), 1)
	}
	mustLocked(t, l, "mo@example.com", "203.0.113.200", "account")
}

func TestLimiterStoresNoEmails(t *testing.T) {
	l, rdb := newTestLimiter(t)
	fail(t, l, "Mo@Example.com ", "203.0.113.7", 1)
	keys, err := rdb.Keys(context.Background(), "*").Result()
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("keys = %v, want 3", keys)
	}
	for _, k := range keys {
		if strings.Contains(strings.ToLower(k), "example.com") {
			t.Errorf("key %q contains the email", k)
		}
		if ttl := rdb.TTL(context.Background(), k).Val(); ttl <= 0 {
			t.Errorf("key %q has no expiry", k)
		}
	}
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "")
	fail(t, l, "mo@example.com", "203.0.113.7", AccountIPLimit-1)
	mustLocked(t, l, "MO@example.com", "203.0.113.7", "account_ip")
}

func TestLimiterUnlock(t *testing.T) {
	l, _ := newTestLimiter(t)
	fail(t, l, "mo@example.com", "203.0.113.7", AccountIPLimit)
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "account_ip")
	if err := l.Unlock(context.Background(), "mo@example.com", "203.0.113.7"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "")
}

func TestLimiterNilAllowsEverything(t *testing.T) {
	var l *Limiter
	mustLocked(t, l, "mo@example.com", "203.0.113.7", "")
	if err := l.Fail(context.Background(), "mo@example.com", "203.0.113.7"); err != nil {
		t.Errorf("Fail: %v", err)
	}
	if err := l.Unlock(context.Background(), "mo@example.com", ""); err == nil {
		t.Error("Unlock on a nil limiter returned nil, want an error")
	}
}

func TestLimiterUnreachable(t *testing.T) {
	l := NewLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond}))
	if _, _, err := l.Locked(context.Background(), "mo@example.com", "203.0.113.7"); err == nil {
		t.Error("Locked with no Redis returned nil, want an error")
	}
}

func newLimitedAPI(t *testing.T, l *Limiter) (http.Handler, *Service) {
	t.Helper()
	s, _ := newTestService(t)
	mux := http.NewServeMux()
	NewHandler(s).WithLimiter(l).Routes(mux)
	return &testAPI{Protect(mux), s}, s
}

func loginFrom(h http.Handler, email, pw, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"`+email+`","password":"`+pw+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderClientIP, ip)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mustTooMany(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d %s, want 429", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "too_many_attempts" {
		t.Errorf("error = %q, want too_many_attempts", got)
	}
	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || secs <= 0 || secs > int(AccountIPWindow.Seconds()) {
		t.Errorf("Retry-After = %q, want 1..%d seconds", rec.Header().Get("Retry-After"), int(AccountIPWindow.Seconds()))
	}
}

func TestLoginRateLimited(t *testing.T) {
	l, _ := newTestLimiter(t)
	h, s := newLimitedAPI(t, l)
	newUser(t, s, "mo@example.com", RoleUser)

	for i := range AccountIPLimit {
		if rec := loginFrom(h, "mo@example.com", "wrong password!", "203.0.113.7"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, rec.Code)
		}
	}
	mustTooMany(t, loginFrom(h, "mo@example.com", "wrong password!", "203.0.113.7"))
	mustTooMany(t, loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7"))

	if rec := loginFrom(h, "mo@example.com", goodPassword, "198.51.100.9"); rec.Code != http.StatusNoContent {
		t.Errorf("from another IP = %d, want 204", rec.Code)
	}
	if err := l.Unlock(context.Background(), "mo@example.com", "203.0.113.7"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if rec := loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7"); rec.Code != http.StatusNoContent {
		t.Errorf("after unlock = %d, want 204", rec.Code)
	}
}

func TestLoginRateLimitUnknownEmailLooksTheSame(t *testing.T) {
	l, _ := newTestLimiter(t)
	h, _ := newLimitedAPI(t, l)
	for range AccountIPLimit {
		if rec := loginFrom(h, "nobody@example.com", "wrong password!", "203.0.113.7"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	}
	mustTooMany(t, loginFrom(h, "nobody@example.com", "wrong password!", "203.0.113.7"))
}

func TestLoginSuccessResetsCounter(t *testing.T) {
	l, _ := newTestLimiter(t)
	h, s := newLimitedAPI(t, l)
	newUser(t, s, "mo@example.com", RoleUser)

	for range AccountIPLimit - 1 {
		loginFrom(h, "mo@example.com", "wrong password!", "203.0.113.7")
	}
	if rec := loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7"); rec.Code != http.StatusNoContent {
		t.Fatalf("correct password = %d, want 204", rec.Code)
	}
	for range AccountIPLimit - 1 {
		loginFrom(h, "mo@example.com", "wrong password!", "203.0.113.7")
	}
	if rec := loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7"); rec.Code != http.StatusNoContent {
		t.Errorf("after reset = %d, want 204", rec.Code)
	}
}

func TestWrongTOTPCodesCount(t *testing.T) {
	l, _ := newTestLimiter(t)
	h, s := newLimitedAPI(t, l)
	u := newUser(t, s, "mo@example.com", RoleUser)
	enableTOTPFor(t, s, u)

	rec := loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7")
	if rec.Code != http.StatusOK {
		t.Fatalf("password step = %d, want 200", rec.Code)
	}
	mfa := cookie(rec, MFACookieName)
	for i := range AccountIPLimit {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login/totp", strings.NewReader(`{"code":"000000"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(HeaderClientIP, "203.0.113.7")
		req.AddCookie(&http.Cookie{Name: MFACookieName, Value: mfa.Value})
		r := httptest.NewRecorder()
		h.ServeHTTP(r, req)
		if r.Code != http.StatusBadRequest {
			t.Fatalf("wrong code %d = %d %s, want 400", i+1, r.Code, r.Body)
		}
	}
	mustTooMany(t, loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7"))
}

func TestLoginFailsOpenWithoutRedis(t *testing.T) {
	l := NewLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond}))
	h, s := newLimitedAPI(t, l)
	newUser(t, s, "mo@example.com", RoleUser)

	for range AccountIPLimit + 1 {
		if rec := loginFrom(h, "mo@example.com", "wrong password!", "203.0.113.7"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password = %d, want 401", rec.Code)
		}
	}
	if rec := loginFrom(h, "mo@example.com", goodPassword, "203.0.113.7"); rec.Code != http.StatusNoContent {
		t.Errorf("correct password = %d, want 204 (fail open)", rec.Code)
	}
}
