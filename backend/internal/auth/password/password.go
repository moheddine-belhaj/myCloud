// Package password hashes and verifies user passwords with argon2id.
//
// Hashes are stored in the PHC string format, which carries its own
// parameters and salt:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
//
// so the parameters can be raised later without breaking existing hashes.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Length limits, counted in characters (runes), not bytes.
const (
	MinLength = 12
	MaxLength = 128
)

var (
	// ErrLength means the password is shorter than MinLength or longer than MaxLength.
	ErrLength = fmt.Errorf("password must be %d to %d characters", MinLength, MaxLength)
	// ErrInvalidHash means a stored hash is not a PHC argon2id string this package understands.
	ErrInvalidHash = errors.New("invalid argon2id hash")
)

// Params are the argon2id cost settings.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32 // bytes
	KeyLength   uint32 // bytes
}

// DefaultParams follow the OWASP recommendation for argon2id:
// 19 MiB of memory, 2 iterations, 1 degree of parallelism.
var DefaultParams = Params{Memory: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}

// maxMemory caps the memory a stored hash may ask for (256 MiB), so a
// tampered row can't make Verify allocate unbounded memory.
const maxMemory = 256 * 1024

// Hasher hashes and verifies passwords. Each argon2id run allocates
// Params.Memory, so at most maxConcurrent run at the same time; a burst of
// requests waits instead of getting the pod OOM-killed.
type Hasher struct {
	params Params
	sem    chan struct{}
}

// NewHasher returns a Hasher that runs at most maxConcurrent hashes at once.
func NewHasher(p Params, maxConcurrent int) *Hasher {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Hasher{params: p, sem: make(chan struct{}, maxConcurrent)}
}

// CheckLength reports ErrLength when pw is outside MinLength..MaxLength.
// It is cheap, so call it before Hash.
func CheckLength(pw string) error {
	if n := utf8.RuneCountInString(pw); n < MinLength || n > MaxLength {
		return ErrLength
	}
	return nil
}

// Hash returns the PHC-encoded argon2id hash of pw with a fresh random salt.
// It waits for a free slot and gives up when ctx is done.
func (h *Hasher) Hash(ctx context.Context, pw string) (string, error) {
	if err := CheckLength(pw); err != nil {
		return "", err
	}
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key, err := h.derive(ctx, pw, salt, h.params)
	if err != nil {
		return "", err
	}
	return encode(h.params, salt, key), nil
}

// Verify reports whether pw matches encoded. A wrong password returns
// (false, nil); a malformed hash returns ErrInvalidHash.
func (h *Hasher) Verify(ctx context.Context, pw, encoded string) (bool, error) {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return false, err
	}
	got, err := h.derive(ctx, pw, salt, p)
	if err != nil {
		return false, err
	}
	// Constant time: the comparison must not reveal how many bytes matched.
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func (h *Hasher) derive(ctx context.Context, pw string, salt []byte, p Params) ([]byte, error) {
	select {
	case h.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-h.sem }()
	return argon2.IDKey([]byte(pw), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength), nil
}

func encode(p Params, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism, b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decode(s string) (Params, []byte, []byte, error) {
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Params{}, nil, nil, ErrInvalidHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if p.Memory == 0 || p.Memory > maxMemory || p.Iterations == 0 || p.Parallelism == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLength, p.KeyLength = uint32(len(salt)), uint32(len(key))
	return p, salt, key, nil
}
