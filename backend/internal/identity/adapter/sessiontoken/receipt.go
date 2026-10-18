package sessiontoken

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"

	"github.com/google/uuid"
)

// ReceiptProtector encrypts refresh recovery material with an independent key.
type ReceiptProtector struct{ aead cipher.AEAD }

// NewReceiptProtector accepts a dedicated 256-bit receipt key.
func NewReceiptProtector(key []byte) (*ReceiptProtector, error) {
	if len(key) != 32 {
		return nil, errors.New("receipt key must contain 32 bytes")
	}
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	return &ReceiptProtector{a}, nil
}
func aad(s uuid.UUID, g int64, o uuid.UUID) []byte {
	b := []byte("glowloom.refresh.receipt.v2\x00")
	b = append(b, s[:]...)
	b = binary.BigEndian.AppendUint64(b, uint64(g))
	return append(b, o[:]...)
}

// Seal binds ciphertext to session, generation and operation.
func (p *ReceiptProtector) Seal(s uuid.UUID, g int64, o uuid.UUID, raw []byte) ([]byte, error) {
	if s == uuid.Nil || o == uuid.Nil || g < 0 {
		return nil, errors.New("invalid receipt binding")
	}
	n := make([]byte, p.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return nil, e
	}
	return p.aead.Seal(n, n, raw, aad(s, g, o)), nil
}

// Open rejects ciphertext with a different recovery binding.
func (p *ReceiptProtector) Open(s uuid.UUID, g int64, o uuid.UUID, sealed []byte) ([]byte, error) {
	n := p.aead.NonceSize()
	if s == uuid.Nil || o == uuid.Nil || g < 0 || len(sealed) < n {
		return nil, errors.New("invalid receipt")
	}
	return p.aead.Open(nil, sealed[:n], sealed[n:], aad(s, g, o))
}
