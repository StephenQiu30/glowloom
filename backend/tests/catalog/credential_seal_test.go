package catalog_test

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
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialseal"
)

func testCredentialKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal test public key: %v", err)
	}
	return privateKey, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
}

func TestCredentialSealUsesDocumentedEnvelopeAndUUIDAAD(t *testing.T) {
	privateKey, publicPEM := testCredentialKey(t)
	sealer, err := credentialseal.NewSealer("agent-2026", publicPEM)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	if sealer.KeyID() != "agent-2026" {
		t.Fatalf("unexpected key ID: %q", sealer.KeyID())
	}
	providerID, credentialID := uuid.New(), uuid.New()
	secret := json.RawMessage(`{"api_key":"local-test-value"}`)
	envelope, err := sealer.Seal(providerID, credentialID, secret)
	if err != nil {
		t.Fatalf("seal credential: %v", err)
	}
	if bytes.Contains(envelope, []byte("local-test-value")) {
		t.Fatal("envelope contains plaintext")
	}
	wrappedLength := privateKey.Size()
	if len(envelope) < wrappedLength+12+16 {
		t.Fatalf("short envelope: %d", len(envelope))
	}
	dek, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, envelope[:wrappedLength], nil)
	if err != nil || len(dek) != 32 {
		t.Fatalf("unwrap DEK: length %d, error %v", len(dek), err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		t.Fatalf("AES key: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("GCM: %v", err)
	}
	aad := make([]byte, 32)
	copy(aad[:16], providerID[:])
	copy(aad[16:], credentialID[:])
	nonce := envelope[wrappedLength : wrappedLength+12]
	ciphertext := envelope[wrappedLength+12:]
	opened, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil || !bytes.Equal(opened, secret) {
		t.Fatalf("open envelope: plaintext matched %t, error %v", bytes.Equal(opened, secret), err)
	}
	aad[0] ^= 1
	if _, err := gcm.Open(nil, nonce, ciphertext, aad); err == nil {
		t.Fatal("envelope accepted a different provider ID")
	}
	envelope[len(envelope)-1] ^= 1
	aad[0] ^= 1
	if _, err := gcm.Open(nil, nonce, envelope[wrappedLength+12:], aad); err == nil {
		t.Fatal("envelope accepted modified ciphertext")
	}
}

func TestCredentialSealRejectsInvalidInput(t *testing.T) {
	_, publicPEM := testCredentialKey(t)
	if _, err := credentialseal.NewSealer("", publicPEM); !errors.Is(err, credentialseal.ErrInvalidKey) {
		t.Fatalf("empty key ID: %v", err)
	}
	if _, err := credentialseal.NewSealer("agent-2026", []byte("bad key")); !errors.Is(err, credentialseal.ErrInvalidKey) {
		t.Fatalf("invalid public key: %v", err)
	}
	weakKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate weak test key: %v", err)
	}
	weakDER, err := x509.MarshalPKIXPublicKey(&weakKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal weak public key: %v", err)
	}
	weakPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: weakDER})
	if _, err := credentialseal.NewSealer("agent-2026", weakPEM); !errors.Is(err, credentialseal.ErrInvalidKey) {
		t.Fatalf("weak public key accepted: %v", err)
	}
	sealer, err := credentialseal.NewSealer("agent-2026", publicPEM)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	for _, input := range []struct {
		providerID, credentialID uuid.UUID
		secret                   json.RawMessage
	}{
		{uuid.Nil, uuid.New(), json.RawMessage(`{"api_key":"test"}`)},
		{uuid.New(), uuid.Nil, json.RawMessage(`{"api_key":"test"}`)},
		{uuid.New(), uuid.New(), json.RawMessage(`[]`)},
		{uuid.New(), uuid.New(), json.RawMessage(`{}`)},
		{uuid.New(), uuid.New(), json.RawMessage(`{"api_key":`)},
	} {
		if _, err := sealer.Seal(input.providerID, input.credentialID, input.secret); !errors.Is(err, credentialseal.ErrInvalidSecret) {
			t.Fatalf("invalid credential accepted: %v", err)
		}
	}
}
