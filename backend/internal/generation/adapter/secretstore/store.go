package secretstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

const FixedRootKeyPath = "/run/secrets/lanverse_media_provider_master_key"

var (
	ErrUnavailable = errors.New("Provider secret store is unavailable")
	ErrDecrypt     = errors.New("Provider secret cannot be decrypted")
)

type Store struct {
	aead           cipher.AEAD
	fingerprintKey [sha256.Size]byte
	keyID          string
}

type associatedData struct {
	WorkspaceID  string `json:"workspace_id"`
	ProviderKey  string `json:"provider_key"`
	CredentialID string `json:"credential_id"`
	Revision     int64  `json:"revision"`
	KeyID        string `json:"key_id"`
}

func OpenFixed() *Store { return Open(FixedRootKeyPath) }

func Open(path string) *Store {
	root, err := os.ReadFile(path)
	if err != nil {
		return &Store{}
	}
	defer wipe(root)
	if len(root) != 32 {
		return &Store{}
	}
	encryptionKey := derive(root, "lanverse/media-provider/encryption/aes-256-gcm")
	fingerprintKey := derive(root, "lanverse/media-provider/fingerprint/hmac-sha256")
	block, err := aes.NewCipher(encryptionKey[:])
	if err != nil {
		return &Store{}
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return &Store{}
	}
	rootHash := sha256.Sum256(root)
	return &Store{aead: aead, fingerprintKey: fingerprintKey, keyID: hex.EncodeToString(rootHash[:8])}
}

func (store *Store) Available() bool {
	return store != nil && store.aead != nil && store.keyID != ""
}

func (store *Store) MatchesKeyID(keyID string) bool {
	return store.Available() && keyID == store.keyID
}

func (store *Store) Encrypt(
	ctx context.Context,
	secretContext domain.ProviderSecretContext,
	plaintext []byte,
) (domain.EncryptedProviderSecret, error) {
	if !store.Available() {
		return domain.EncryptedProviderSecret{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.EncryptedProviderSecret{}, err
	}
	secretContext.KeyID = store.keyID
	aad, err := encodeAssociatedData(secretContext)
	if err != nil {
		return domain.EncryptedProviderSecret{}, err
	}
	nonce := make([]byte, store.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return domain.EncryptedProviderSecret{}, err
	}
	ciphertext := store.aead.Seal(nil, nonce, plaintext, aad)
	fingerprint, err := store.Fingerprint(ctx, plaintext)
	if err != nil {
		return domain.EncryptedProviderSecret{}, err
	}
	return domain.EncryptedProviderSecret{
		CipherSuite: domain.ProviderCipherAES256GCM,
		KeyID:       store.keyID,
		Nonce:       append([]byte(nil), nonce...),
		Ciphertext:  ciphertext,
		Fingerprint: fingerprint,
	}, nil
}

func (store *Store) Fingerprint(ctx context.Context, plaintext []byte) (string, error) {
	if !store.Available() {
		return "", ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fingerprint := hmac.New(sha256.New, store.fingerprintKey[:])
	_, _ = fingerprint.Write(plaintext)
	return hex.EncodeToString(fingerprint.Sum(nil)), nil
}

func (store *Store) Decrypt(
	ctx context.Context,
	secretContext domain.ProviderSecretContext,
	encrypted domain.EncryptedProviderSecret,
) ([]byte, error) {
	if !store.Available() {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if encrypted.CipherSuite != domain.ProviderCipherAES256GCM || encrypted.KeyID != store.keyID ||
		len(encrypted.Nonce) != store.aead.NonceSize() || len(encrypted.Ciphertext) < store.aead.Overhead() ||
		(secretContext.KeyID != "" && secretContext.KeyID != encrypted.KeyID) {
		return nil, ErrDecrypt
	}
	secretContext.KeyID = encrypted.KeyID
	aad, err := encodeAssociatedData(secretContext)
	if err != nil {
		return nil, ErrDecrypt
	}
	plaintext, err := store.aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	fingerprint, fingerprintErr := store.Fingerprint(ctx, plaintext)
	expectedFingerprint, decodeErr := hex.DecodeString(encrypted.Fingerprint)
	actualFingerprint, actualDecodeErr := hex.DecodeString(fingerprint)
	if fingerprintErr != nil || decodeErr != nil || actualDecodeErr != nil ||
		!hmac.Equal(actualFingerprint, expectedFingerprint) {
		wipe(plaintext)
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

func wipe(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func derive(root []byte, label string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, root)
	_, _ = mac.Write([]byte(label))
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func encodeAssociatedData(value domain.ProviderSecretContext) ([]byte, error) {
	if value.WorkspaceID == "" || value.ProviderKey == "" || value.CredentialID == "" ||
		value.Revision < 1 || value.KeyID == "" {
		return nil, errors.New("Provider secret context is invalid")
	}
	return json.Marshal(associatedData{
		WorkspaceID:  value.WorkspaceID,
		ProviderKey:  value.ProviderKey,
		CredentialID: value.CredentialID,
		Revision:     value.Revision,
		KeyID:        value.KeyID,
	})
}
