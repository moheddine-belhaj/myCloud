// Package totp implements time-based one-time passwords (RFC 6238) as used
// by authenticator apps: HMAC-SHA1, 6 digits, a new code every 30 seconds.
//
// It only does the maths. Storing the secret (encrypted) and remembering the
// last used time step (so a code can't be used twice) is the caller's job.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default; every authenticator app supports it
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Digits is the code length.
	Digits = 6
	// Period is how long one code is valid.
	Period = 30 * time.Second
	// Skew is how many steps before and after "now" are accepted, to allow
	// for clock drift and slow typing: 1 step = ±30 s.
	Skew = 1
	// SecretSize is the secret length in bytes: 160 bits, as RFC 4226 recommends.
	SecretSize = 20
)

// b32 is the encoding authenticator apps expect: base32 without padding.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random secret.
func NewSecret() []byte {
	s := make([]byte, SecretSize)
	if _, err := rand.Read(s); err != nil {
		panic(fmt.Sprintf("totp: crypto/rand failed: %v", err)) // never happens on Linux
	}
	return s
}

// EncodeSecret returns the secret as base32, the form users type into an app
// when they can't scan a QR code.
func EncodeSecret(secret []byte) string { return b32.EncodeToString(secret) }

// URI returns the otpauth:// URI that a QR code carries. issuer shows up as
// the app's account title ("MyCloud"), account as its subtitle (the email).
// Key URI format: https://github.com/google/google-authenticator/wiki/Key-Uri-Format
func URI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", EncodeSecret(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period/time.Second)))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Step returns the time step for t: the number of 30 s periods since 1970.
func Step(t time.Time) int64 { return t.Unix() / int64(Period/time.Second) }

// Code returns the code for a time step.
func Code(secret []byte, step int64) string { return hotp(secret, step, Digits) }

// hotp is RFC 4226: HMAC the step, take 4 bytes at an offset given by the
// last nibble ("dynamic truncation"), keep the last n decimal digits.
func hotp(secret []byte, counter int64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for range digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

// Validate checks code against the steps around now and returns the step it
// matched. Steps at or before lastStep are refused, so each code works once
// (pass 0 when nothing was used yet). Spaces in the code are ignored
// ("123 456" as some apps show it).
func Validate(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != Digits || strings.Trim(code, "0123456789") != "" {
		return 0, false
	}
	cur := Step(now)
	// Check every candidate, so the time taken doesn't depend on which
	// step matched.
	var matched int64
	found := 0
	for step := cur - Skew; step <= cur+Skew; step++ {
		eq := subtle.ConstantTimeCompare([]byte(Code(secret, step)), []byte(code))
		if eq == 1 && step > lastStep && found == 0 {
			matched, found = step, 1
		}
	}
	return matched, found == 1
}
