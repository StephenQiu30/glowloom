// Package credentialseal encrypts provider secrets for the Agent's public key.
package credentialseal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	// ErrInvalidKey means the Agent public key or its identifier is unusable.
	ErrInvalidKey = errors.New("invalid credential sealing key")
	// ErrInvalidSecret means the credential identity or JSON object is invalid.
	ErrInvalidSecret = errors.New("invalid provider secret")
)

// Sealer has only the public key; it cannot recover stored provider secrets.
type Sealer struct {
	keyID     string
	publicKey *rsa.PublicKey
}

// NewSealer accepts one PEM PKIX RSA public key of at least 2048 bits.
func NewSealer(keyID string, publicKeyPEM []byte) (*Sealer, error) {
	if len(bytes.TrimSpace([]byte(keyID))) == 0 || len(keyID) > 128 {
		return nil, ErrInvalidKey
	}
	block, rest := pem.Decode(publicKeyPEM)
	if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, ErrInvalidKey
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, ErrInvalidKey
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok || publicKey.Size() < 256 {
		return nil, ErrInvalidKey
	}
	return &Sealer{keyID: keyID, publicKey: publicKey}, nil
}

// KeyID identifies the Agent private key needed to open a sealed credential.
func (s *Sealer) KeyID() string { return s.keyID }

// Seal returns wrapped DEK || 12-byte nonce || AES-GCM ciphertext and tag.
func (s *Sealer) Seal(providerID, credentialID uuid.UUID, secret json.RawMessage) ([]byte, error) {
	if providerID == uuid.Nil || credentialID == uuid.Nil {
		return nil, ErrInvalidSecret
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(secret, &object); err != nil || len(object) == 0 {
		return nil, ErrInvalidSecret
	}
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("generate credential encryption key: %w", err)
	}
	defer clear(dek)
	wrappedDEK, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, s.publicKey, dek, nil)
	if err != nil {
		return nil, fmt.Errorf("wrap credential encryption key: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	var aad [32]byte
	copy(aad[:16], providerID[:])
	copy(aad[16:], credentialID[:])
	envelope := make([]byte, 0, len(wrappedDEK)+len(nonce)+len(secret)+gcm.Overhead())
	envelope = append(envelope, wrappedDEK...)
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, secret, aad[:])
	return envelope, nil
}
