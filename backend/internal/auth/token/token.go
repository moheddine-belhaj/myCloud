// Package token creates the random secrets MyCloud hands out (invite codes, password reset tokens, etc.) and hashes them for storage in the database.
//
// Only Hash(secret) goes into the database. The secrets are 256-bit random
// values, so a plain SHA-256 is enough: there is nothing to brute-force, which
// is why passwords (chosen by people) need argon2id and these don't.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// size is the number of random bytes in a secret (256 bits).
const size = 32

// New returns a new random secret, URL-safe base64 without padding (43 characters).
func New() string {
	b := make([]byte, size)
	rand.Read(b) // never returns an error; it crashes the program if the OS has no entropy
	return base64.RawURLEncoding.EncodeToString(b)
}

// Hash returns the SHA-256 of secret, the value stored in the database.
func Hash(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
