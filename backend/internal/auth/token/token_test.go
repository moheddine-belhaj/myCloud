package token

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestNew(t *testing.T) {
	a, b := New(), New()
	if a == b {
		t.Fatal("two tokens are equal")
	}
	raw, err := base64.RawURLEncoding.DecodeString(a)
	if err != nil {
		t.Fatalf("token is not URL-safe base64: %v", err)
	}
	if len(raw) != 32 || len(a) != 43 {
		t.Errorf("token = %d chars / %d bytes, want 43 / 32", len(a), len(raw))
	}
}

func TestHash(t *testing.T) {
	s := New()
	if !bytes.Equal(Hash(s), Hash(s)) {
		t.Error("Hash is not deterministic")
	}
	if len(Hash(s)) != 32 {
		t.Errorf("len(Hash) = %d, want 32", len(Hash(s)))
	}
	if bytes.Equal(Hash(s), Hash(New())) {
		t.Error("different tokens have the same hash")
	}
}
