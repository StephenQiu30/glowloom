package credentialseal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"

	"github.com/google/uuid"
)

// Unsealer belongs only to the provider worker, never the API process.
type Unsealer struct {
	keyID      string
	privateKey *rsa.PrivateKey
}

// NewUnsealer parses a PKCS8 RSA key of at least 2048 bits.
func NewUnsealer(keyID string, data []byte) (*Unsealer, error) {
	block, rest := pem.Decode(data)
	if keyID == "" || block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, ErrInvalidKey
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ErrInvalidKey
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.Size() < 256 || key.Validate() != nil {
		return nil, ErrInvalidKey
	}
	return &Unsealer{keyID: keyID, privateKey: key}, nil
}

// Open authenticates the provider/credential binding before releasing plaintext.
func (u *Unsealer) Open(keyID string, providerID, credentialID uuid.UUID, envelope []byte) ([]byte, error) {
	if u == nil || keyID != u.keyID || providerID == uuid.Nil || credentialID == uuid.Nil || len(envelope) > 16384 || len(envelope) < u.privateKey.Size()+12+16 {
		return nil, ErrInvalidSecret
	}
	wrapped := u.privateKey.Size()
	dek, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, u.privateKey, envelope[:wrapped], nil)
	if err != nil {
		return nil, ErrInvalidSecret
	}
	defer clear(dek)
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, ErrInvalidSecret
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidSecret
	}
	var aad [32]byte
	copy(aad[:16], providerID[:])
	copy(aad[16:], credentialID[:])
	plaintext, err := gcm.Open(nil, envelope[wrapped:wrapped+12], envelope[wrapped+12:], aad[:])
	if err != nil {
		return nil, ErrInvalidSecret
	}
	return plaintext, nil
}
