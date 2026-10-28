// Package keyring encrypts small secrets at rest (the TOTP seeds of M3) with
// AES-256-GCM.
//
// The key never lives in the database or in Git: it comes from Vault through
// External Secrets Operator as an environment variable. A database dump alone
// is therefore useless for generating codes.
//
// Ciphertext layout (all stored together in one bytea column):
//
//	version (1 byte) | key ID (1 byte) | nonce (12 bytes) | ciphertext + tag
//
// The key ID makes key rotation possible later: new rows use the current key,
// old rows still name the key that sealed them.
package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = 32

	formatV1   = 1
	headerSize = 2 // version + key ID
)

var (
	// ErrMalformed means the blob is too short or has an unknown format version.
	ErrMalformed = errors.New("keyring: malformed ciphertext")
	// ErrUnknownKey means the blob was sealed with a key this keyring doesn't hold.
	ErrUnknownKey = errors.New("keyring: unknown key ID")
	// ErrDecrypt means authentication failed: wrong key, wrong associated data
	// or a modified blob. The reasons are deliberately not told apart.
	ErrDecrypt = errors.New("keyring: decryption failed")
)

// Keyring seals with its current key and opens blobs sealed with any key it holds.
type Keyring struct {
	current byte
	aeads   map[byte]cipher.AEAD
}

// New returns a keyring whose current key is key, identified by id.
// id must not be 0, so a zeroed blob can never match a key.
func New(id byte, key []byte) (*Keyring, error) {
	if id == 0 {
		return nil, errors.New("keyring: key ID must not be 0")
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &Keyring{current: id, aeads: map[byte]cipher.AEAD{id: aead}}, nil
}

// Parse is New for a base64-encoded key, as delivered in an environment
// variable. Surrounding whitespace (a trailing newline) is ignored.
func Parse(id byte, b64 string) (*Keyring, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, errors.New("keyring: key is not valid base64")
	}
	return New(id, key)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		// Never include the key itself in the error.
		return nil, fmt.Errorf("keyring: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("keyring: %w", err)
	}
	return cipher.NewGCM(block)
}

// CurrentID is the ID of the key used by Seal. Safe to log.
func (k *Keyring) CurrentID() byte { return k.current }

// Seal encrypts plaintext with the current key and a fresh random nonce.
// aad (associated data) is authenticated but not stored: pass the same value
// to Open. Binding a seed to its user ID stops a seed from being copied to
// another user's row.
func (k *Keyring) Seal(plaintext, aad []byte) ([]byte, error) {
	aead := k.aeads[k.current]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("keyring: nonce: %w", err)
	}
	out := make([]byte, 0, headerSize+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, formatV1, k.current)
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plaintext, aad), nil
}

// Open decrypts a blob made by Seal, with the key named in its header.
func (k *Keyring) Open(blob, aad []byte) ([]byte, error) {
	if len(blob) < headerSize || blob[0] != formatV1 {
		return nil, ErrMalformed
	}
	aead, ok := k.aeads[blob[1]]
	if !ok {
		return nil, ErrUnknownKey
	}
	rest := blob[headerSize:]
	if len(rest) < aead.NonceSize()+aead.Overhead() {
		return nil, ErrMalformed
	}
	nonce, ciphertext := rest[:aead.NonceSize()], rest[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
