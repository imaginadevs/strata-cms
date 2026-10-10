// Package crypto provides symmetric encryption-at-rest for small secrets stored
// in the database (e.g. a tenant's S3 secret key).
//
// Values are AES-256-GCM encrypted and tagged with a version marker so plaintext
// written before encryption was enabled is detected and passed through unchanged
// (it gets encrypted the next time it is saved). The key comes from config/env.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
)

const marker = "enc:v1:"

// Encryptor seals/opens short secret strings. A zero/disabled Encryptor is a
// pass-through (no encryption) so the app still runs if no key is configured.
type Encryptor struct {
	aead    cipher.AEAD
	enabled bool
}

// New derives a 256-bit key from the given passphrase (any length) and returns an
// Encryptor. An empty key yields a disabled (pass-through) Encryptor.
func New(key string) *Encryptor {
	if key == "" {
		return &Encryptor{}
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return &Encryptor{}
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return &Encryptor{}
	}
	return &Encryptor{aead: g, enabled: true}
}

// Enabled reports whether encryption is active.
func (e *Encryptor) Enabled() bool { return e.enabled }

// Encrypt returns a marked, base64 ciphertext. It is idempotent (already-encrypted
// or empty input is returned unchanged). On the (practically impossible) failure
// to read randomness it returns the plaintext rather than losing the value.
func (e *Encryptor) Encrypt(plain string) string {
	if !e.enabled || plain == "" || strings.HasPrefix(plain, marker) {
		return plain
	}
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plain
	}
	ct := e.aead.Seal(nonce, nonce, []byte(plain), nil)
	return marker + base64.RawStdEncoding.EncodeToString(ct)
}

// Decrypt reverses Encrypt. Unmarked input (legacy plaintext) is returned as-is.
// On any decode/open error the input is returned unchanged (so a wrong key
// surfaces downstream rather than silently emptying the secret).
func (e *Encryptor) Decrypt(s string) string {
	if !strings.HasPrefix(s, marker) {
		return s
	}
	if !e.enabled {
		return s
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, marker))
	if err != nil {
		return s
	}
	ns := e.aead.NonceSize()
	if len(raw) < ns {
		return s
	}
	pt, err := e.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return s
	}
	return string(pt)
}
