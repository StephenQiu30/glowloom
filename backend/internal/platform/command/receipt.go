package command

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrReceiptNotFound = errors.New("command receipt not found")
var ErrInputMismatch = errors.New("idempotency key was already used with different input")

type Receipt struct {
	ID, WorkspaceID, Operation, IdempotencyKey string
	InputHash, ResourceID, CreatedBy           string
	Result                                     json.RawMessage
	CreatedAt                                  time.Time
}

func InputHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode command input: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func Result(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode command result: %w", err)
	}
	return encoded, nil
}

func Replay[T any](receipt Receipt, expectedInputHash string) (T, error) {
	var result T
	if receipt.InputHash != expectedInputHash {
		return result, ErrInputMismatch
	}
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		return result, fmt.Errorf("decode command result: %w", err)
	}
	return result, nil
}
