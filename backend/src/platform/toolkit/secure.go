package toolkit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// RandomHexToken returns a cryptographically random opaque value encoded as
// lowercase hexadecimal. Callers persist only a digest when the value is a
// credential or a one-time secret.
func RandomHexToken(byteLength int) (string, error) {
	if byteLength <= 0 || byteLength > 4096 {
		return "", errors.New("random token length must be between 1 and 4096 bytes")
	}
	raw := make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func SHA256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func SHA256String(value string) string {
	return SHA256Hex([]byte(value))
}
