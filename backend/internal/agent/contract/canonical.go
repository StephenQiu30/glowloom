package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Executor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Model   string `json:"model"`
}

type ResultError struct {
	Code      string `json:"code"`
	Summary   string `json:"summary"`
	Retryable bool   `json:"retryable"`
}

func CanonicalHash(raw json.RawMessage) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	hash := sha256.Sum256(bytes.TrimSpace(canonical.Bytes()))
	return hex.EncodeToString(hash[:]), nil
}

func jsonObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return false
	}
	var value map[string]any
	return json.Unmarshal(trimmed, &value) == nil
}
