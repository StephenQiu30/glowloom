// Package credentialproof protects short-lived email proofs and queued mail.
package credentialproof

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"github.com/google/uuid"
)

var ErrInvalidEnvelope = errors.New("invalid credential envelope")

// Protector keeps separate proof and envelope keys outside the database.
type Protector struct {
	keyID    string
	proofKey []byte
	aead     cipher.AEAD
}

func New(keyID string, proofKey, encryptionKey []byte) (*Protector, error) {
	if keyID == "" || len(proofKey) != 32 || len(encryptionKey) != 32 || hmac.Equal(proofKey, encryptionKey) {
		return nil, errors.New("independent 32-byte identity keys and key ID required")
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Protector{keyID: keyID, proofKey: append([]byte(nil), proofKey...), aead: aead}, nil
}

func (p *Protector) KeyID() string { return p.keyID }

// Digest binds the proof to its purpose and immutable operation identity.
func (p *Protector) Digest(purpose string, id uuid.UUID, secret string) []byte {
	h := hmac.New(sha256.New, p.proofKey)
	_, _ = h.Write([]byte(purpose))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(id[:])
	_, _ = h.Write([]byte(secret))
	return h.Sum(nil)
}

func (p *Protector) Seal(id uuid.UUID, payload []byte) ([]byte, error) {
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return p.aead.Seal(nonce, nonce, payload, id[:]), nil
}

func (p *Protector) Open(id uuid.UUID, keyID string, envelope []byte) ([]byte, error) {
	if keyID != p.keyID || len(envelope) < p.aead.NonceSize()+p.aead.Overhead() {
		return nil, ErrInvalidEnvelope
	}
	plain, err := p.aead.Open(nil, envelope[:p.aead.NonceSize()], envelope[p.aead.NonceSize():], id[:])
	if err != nil {
		return nil, ErrInvalidEnvelope
	}
	return plain, nil
}
