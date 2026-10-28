package keyring

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func testKeyring(t *testing.T) *Keyring {
	t.Helper()
	kr, err := New(1, testKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return kr
}

func TestRoundTrip(t *testing.T) {
	kr := testKeyring(t)
	seed, aad := []byte("JBSWY3DPEHPK3PXP"), []byte("user-1")

	blob, err := kr.Seal(seed, aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(blob, seed) {
		t.Fatal("blob contains the plaintext")
	}
	if blob[0] != formatV1 || blob[1] != 1 {
		t.Errorf("header = %v, want [1 1]", blob[:2])
	}
	got, err := kr.Open(blob, aad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, seed) {
		t.Errorf("Open = %q, want %q", got, seed)
	}
}

// Same input sealed twice must give different blobs (fresh nonce each time).
func TestFreshNonce(t *testing.T) {
	kr := testKeyring(t)
	a, _ := kr.Seal([]byte("seed"), nil)
	b, _ := kr.Seal([]byte("seed"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same plaintext are identical")
	}
}

func TestOpenFailures(t *testing.T) {
	kr := testKeyring(t)
	blob, _ := kr.Seal([]byte("seed"), []byte("user-1"))

	other, _ := New(1, testKey(t)) // same ID, different key
	tampered := bytes.Clone(blob)
	tampered[len(tampered)-1] ^= 0x01
	wrongVersion := bytes.Clone(blob)
	wrongVersion[0] = 2
	unknownKey := bytes.Clone(blob)
	unknownKey[1] = 9

	for name, tc := range map[string]struct {
		kr   *Keyring
		blob []byte
		aad  string
		want error
	}{
		"wrong aad (other user)": {kr, blob, "user-2", ErrDecrypt},
		"wrong key":              {other, blob, "user-1", ErrDecrypt},
		"tampered":               {kr, tampered, "user-1", ErrDecrypt},
		"wrong version":          {kr, wrongVersion, "user-1", ErrMalformed},
		"unknown key ID":         {kr, unknownKey, "user-1", ErrUnknownKey},
		"too short":              {kr, blob[:10], "user-1", ErrMalformed},
		"empty":                  {kr, nil, "user-1", ErrMalformed},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.kr.Open(tc.blob, []byte(tc.aad)); !errors.Is(err, tc.want) {
				t.Errorf("Open = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(0, testKey(t)); err == nil {
		t.Error("key ID 0 accepted")
	}
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := New(1, make([]byte, n)); err == nil {
			t.Errorf("%d-byte key accepted", n)
		}
	}
}

func TestParse(t *testing.T) {
	key := testKey(t)
	b64 := base64.StdEncoding.EncodeToString(key)

	kr, err := Parse(1, b64+"\n") // as written by `openssl rand -base64 32`
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	direct, _ := New(1, key)
	blob, _ := kr.Seal([]byte("seed"), nil)
	if _, err := direct.Open(blob, nil); err != nil {
		t.Errorf("Parse and New give different keys: %v", err)
	}

	for name, in := range map[string]string{
		"empty":      "",
		"not base64": "not-base64!!",
		"too short":  base64.StdEncoding.EncodeToString(key[:16]),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(1, in)
			if err == nil {
				t.Fatal("accepted")
			}
			if in != "" && strings.Contains(err.Error(), in) {
				t.Error("error message echoes the key")
			}
		})
	}
}
