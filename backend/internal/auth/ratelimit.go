package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Failed-login limits (M4). See README.md#rate-limiting.
const (
	AccountIPLimit   = 5
	AccountIPWindow  = 15 * time.Minute
	IPLimit          = 20
	IPWindow         = 15 * time.Minute
	AccountLimit     = 50
	AccountWindow    = time.Hour
	rateLimitTimeout = 300 * time.Millisecond
)

type rule struct {
	name   string
	limit  int64
	window time.Duration
	key    func(account, ip string) string
}

var rules = []rule{
	{"account_ip", AccountIPLimit, AccountIPWindow, func(a, ip string) string {
		if a == "" || ip == "" {
			return ""
		}
		return "rl:login:acctip:" + a + ":" + ip
	}},
	{"ip", IPLimit, IPWindow, func(_, ip string) string {
		if ip == "" {
			return ""
		}
		return "rl:login:ip:" + ip
	}},
	{"account", AccountLimit, AccountWindow, func(a, _ string) string {
		if a == "" {
			return ""
		}
		return "rl:login:acct:" + a
	}},
}

// Limiter counts failed logins in Redis. A nil *Limiter allows everything.
type Limiter struct {
	rdb redis.UniversalClient
}

// NewLimiter returns a Limiter backed by rdb.
func NewLimiter(rdb redis.UniversalClient) *Limiter {
	return &Limiter{rdb: rdb}
}

// Locked reports whether email or ip has reached a limit and how long the lock lasts.
// An empty email checks only the IP limit.
func (l *Limiter) Locked(ctx context.Context, email, ip string) (time.Duration, string, error) {
	if l == nil {
		return 0, "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, rateLimitTimeout)
	defer cancel()
	acct := accountKey(email)
	type check struct {
		r     rule
		count *redis.StringCmd
		ttl   *redis.DurationCmd
	}
	var checks []check
	pipe := l.rdb.Pipeline()
	for _, r := range rules {
		if k := r.key(acct, ip); k != "" {
			checks = append(checks, check{r, pipe.Get(ctx, k), pipe.PTTL(ctx, k)})
		}
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, "", fmt.Errorf("rate limit check: %w", err)
	}
	var wait time.Duration
	var hit string
	for _, c := range checks {
		n, err := c.count.Int64()
		if err != nil || n < c.r.limit {
			continue
		}
		d := c.ttl.Val()
		if d <= 0 {
			d = c.r.window
		}
		if d > wait {
			wait, hit = d, c.r.name
		}
	}
	return wait, hit, nil
}

// Fail counts one failed attempt for email and ip on every limit.
func (l *Limiter) Fail(ctx context.Context, email, ip string) error {
	if l == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, rateLimitTimeout)
	defer cancel()
	acct := accountKey(email)
	pipe := l.rdb.TxPipeline()
	for _, r := range rules {
		if k := r.key(acct, ip); k != "" {
			pipe.Incr(ctx, k)
			pipe.ExpireNX(ctx, k, r.window)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("rate limit count: %w", err)
	}
	return nil
}

// Reset clears the account+IP counter after a complete login.
func (l *Limiter) Reset(ctx context.Context, email, ip string) error {
	if l == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, rateLimitTimeout)
	defer cancel()
	if k := rules[0].key(accountKey(email), ip); k != "" {
		if err := l.rdb.Del(ctx, k).Err(); err != nil {
			return fmt.Errorf("rate limit reset: %w", err)
		}
	}
	return nil
}

// Unlock deletes the account counter and, with ip, the account+IP and IP counters.
func (l *Limiter) Unlock(ctx context.Context, email, ip string) error {
	if l == nil {
		return errors.New("rate limiting is not configured (REDIS_ADDR is empty)")
	}
	acct := accountKey(email)
	var keys []string
	for _, r := range rules {
		if k := r.key(acct, ip); k != "" {
			keys = append(keys, k)
		}
	}
	if err := l.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	return nil
}

// accountKey is the SHA-256 of the normalized email, so Redis never holds addresses.
func accountKey(email string) string {
	if email == "" {
		return ""
	}
	e, err := NormalizeEmail(email)
	if err != nil {
		e = strings.ToLower(strings.TrimSpace(email))
	}
	sum := sha256.Sum256([]byte(e))
	return hex.EncodeToString(sum[:])
}
