package password

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// cheap keeps the tests fast; TestDefaultParams covers the real settings once.
var cheap = Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func TestHashAndVerify(t *testing.T) {
	ctx := context.Background()
	h := NewHasher(cheap, 2)

	hash, err := h.Hash(ctx, "correct horse battery")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Errorf("hash = %q, want PHC argon2id prefix", hash)
	}

	ok, err := h.Verify(ctx, "correct horse battery", hash)
	if err != nil || !ok {
		t.Errorf("Verify(right password) = %v, %v; want true, nil", ok, err)
	}
	ok, err = h.Verify(ctx, "wrong horse battery", hash)
	if err != nil || ok {
		t.Errorf("Verify(wrong password) = %v, %v; want false, nil", ok, err)
	}
}

func TestSaltIsRandom(t *testing.T) {
	h := NewHasher(cheap, 1)
	a, _ := h.Hash(context.Background(), "same password 123")
	b, _ := h.Hash(context.Background(), "same password 123")
	if a == b {
		t.Error("two hashes of the same password are equal: salt is not random")
	}
}

func TestVerifyUsesParamsFromHash(t *testing.T) {
	// A hash made with other parameters must still verify after DefaultParams change.
	old, _ := NewHasher(cheap, 1).Hash(context.Background(), "an older password")
	newer := NewHasher(Params{Memory: 128, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}, 1)
	if ok, err := newer.Verify(context.Background(), "an older password", old); err != nil || !ok {
		t.Errorf("Verify(old hash) = %v, %v; want true, nil", ok, err)
	}
}

func TestLength(t *testing.T) {
	h := NewHasher(cheap, 1)
	for _, pw := range []string{"", "short", strings.Repeat("a", 11), strings.Repeat("a", 129)} {
		if _, err := h.Hash(context.Background(), pw); !errors.Is(err, ErrLength) {
			t.Errorf("Hash(%d chars) err = %v, want ErrLength", len(pw), err)
		}
	}
	// 12 runes but 36 bytes: the limit counts characters, not bytes.
	if err := CheckLength(strings.Repeat("ä", 6) + "€€€€€€"); err != nil {
		t.Errorf("CheckLength(12 multi-byte runes) = %v, want nil", err)
	}
	if err := CheckLength(strings.Repeat("a", 128)); err != nil {
		t.Errorf("CheckLength(128) = %v, want nil", err)
	}
}

func TestInvalidHash(t *testing.T) {
	h := NewHasher(cheap, 1)
	for _, s := range []string{
		"",
		"plain",
		"$argon2i$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0$aGFzaA",
		"$argon2id$v=18$m=64,t=1,p=1$c2FsdHNhbHRzYWx0$aGFzaA",
		"$argon2id$v=19$m=999999999,t=1,p=1$c2FsdHNhbHRzYWx0$aGFzaA",
		"$argon2id$v=19$m=64,t=0,p=1$c2FsdHNhbHRzYWx0$aGFzaA",
		"$argon2id$v=19$m=64,t=1,p=1$!!!$aGFzaA",
	} {
		if _, err := h.Verify(context.Background(), "whatever password", s); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Verify(%q) err = %v, want ErrInvalidHash", s, err)
		}
	}
}

func TestConcurrencyLimitHonoursContext(t *testing.T) {
	h := NewHasher(cheap, 1)
	h.sem <- struct{}{} // occupy the only slot
	defer func() { <-h.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "waiting for a slot"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Hash with full semaphore err = %v, want DeadlineExceeded", err)
	}
}

func TestDefaultParams(t *testing.T) {
	h := NewHasher(DefaultParams, 1)
	hash, err := h.Hash(context.Background(), "the real cost settings")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash = %q, want OWASP parameters", hash)
	}
}
