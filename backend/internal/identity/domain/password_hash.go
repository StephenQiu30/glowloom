package domain

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const argonPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"

// HashPassword always produces a versioned Argon2id credential with a fresh salt.
func HashPassword(password, currentHash string) (string, error) {
	if err := ValidatePassword(password, currentHash); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	digest := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return argonPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(digest), nil
}

// VerifyPassword retains bcrypt verification for existing accounts only.
func VerifyPassword(hash, password string) bool {
	if strings.HasPrefix(hash, "$argon2id$") {
		salt, expected, ok := parseArgonHash(hash)
		if !ok || len(password) > 512 {
			return false
		}
		actual := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
		return subtle.ConstantTimeCompare(actual, expected) == 1
	}
	// Bound stored parameters before invoking an expensive verifier.
	if !validLegacyHash(hash) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ValidPasswordHash accepts the current format and the supported legacy format.
func ValidPasswordHash(hash string) bool {
	_, _, ok := parseArgonHash(hash)
	return ok || validLegacyHash(hash)
}

// PasswordNeedsUpgrade is evaluated only after a successful verification.
func PasswordNeedsUpgrade(hash string) bool { return validLegacyHash(hash) }

func validLegacyHash(hash string) bool {
	if len(hash) != 60 {
		return false
	}
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost == 12
}

func parseArgonHash(hash string) ([]byte, []byte, bool) {
	if !strings.HasPrefix(hash, argonPrefix) || len(hash) != len(argonPrefix)+22+1+43 {
		return nil, nil, false
	}
	saltText, digestText, ok := strings.Cut(strings.TrimPrefix(hash, argonPrefix), "$")
	if !ok {
		return nil, nil, false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(saltText)
	if err != nil || len(salt) != 16 {
		return nil, nil, false
	}
	digest, err := base64.RawStdEncoding.Strict().DecodeString(digestText)
	if err != nil || len(digest) != 32 {
		return nil, nil, false
	}
	return salt, digest, true
}
