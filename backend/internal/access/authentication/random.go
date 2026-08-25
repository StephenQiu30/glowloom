package authentication

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
)

func RandomSecret() (string, error) {
	value := make([]byte, 48)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func RandomNumericCode() string {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%06d", value.Int64())
}
