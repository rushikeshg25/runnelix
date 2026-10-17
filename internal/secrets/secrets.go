// Package secrets encrypts stored credentials with scope-bound AES-256-GCM.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

type Box struct {
	aead  cipher.AEAD
	KeyID string
}

func New(keyHex, keyID string) (*Box, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 || keyID == "" {
		return nil, errors.New("credential key must be 32 bytes with a key ID")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead, keyID}, nil
}
func (b *Box) Seal(scope string, value []byte) ([]byte, error) {
	if len(value) == 0 || len(value) > 4096 {
		return nil, errors.New("invalid credential size")
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, value, []byte(scope)), nil
}
func (b *Box) Open(scope, keyID string, value []byte) ([]byte, error) {
	if keyID != b.KeyID || len(value) < b.aead.NonceSize() {
		return nil, errors.New("credential unavailable")
	}
	plain, err := b.aead.Open(nil, value[:b.aead.NonceSize()], value[b.aead.NonceSize():], []byte(scope))
	if err != nil {
		return nil, errors.New("credential unavailable")
	}
	return plain, nil
}
