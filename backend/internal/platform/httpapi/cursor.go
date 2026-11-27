package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

// CursorBinding fingerprints the authorized query without its continuation position.
func CursorBinding(query any) (string, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

// DecodeCursorToken decodes a bounded URL-safe JSON token.
func DecodeCursorToken(token string, destination any) bool {
	if len(token) > 2048 {
		return false
	}
	body, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && json.Unmarshal(body, destination) == nil
}

// EncodeCursorToken encodes a position using the public URL-safe token format.
func EncodeCursorToken(position any) (*string, error) {
	body, err := json.Marshal(position)
	if err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(body)
	return &token, nil
}
