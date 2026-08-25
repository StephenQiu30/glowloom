package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/google/uuid"
)

const SchemaVersion = "agent-candidate-v1"

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Invocation struct {
	InvocationID  string          `json:"invocation_id"`
	Kind          string          `json:"kind"`
	InputHash     string          `json:"input_hash"`
	SchemaVersion string          `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

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

type Result struct {
	InvocationID  string          `json:"invocation_id"`
	Kind          string          `json:"kind"`
	InputHash     string          `json:"input_hash"`
	Status        string          `json:"status"`
	SchemaVersion string          `json:"schema_version"`
	Candidate     json.RawMessage `json:"candidate"`
	ResultHash    *string         `json:"result_hash"`
	Executor      Executor        `json:"executor"`
	Error         *ResultError    `json:"error"`
}

func (value Invocation) Validate() error {
	if _, err := uuid.Parse(value.InvocationID); err != nil {
		return errors.New("invalid agent invocation")
	}
	if !oneOf(value.Kind, "production_bible", "script_structure", "storyboard_draft") || !hashPattern.MatchString(value.InputHash) || value.SchemaVersion != SchemaVersion || !jsonObject(value.Payload) {
		return errors.New("invalid agent invocation")
	}
	return nil
}

func (value Result) ValidateFor(invocation Invocation) error {
	if value.InvocationID != invocation.InvocationID || value.Kind != invocation.Kind || value.InputHash != invocation.InputHash || value.SchemaVersion != SchemaVersion || !oneOf(value.Status, "succeeded", "failed", "unknown") || value.Executor.Name == "" || value.Executor.Version == "" || value.Executor.Model == "" {
		return errors.New("agent result identity does not match invocation")
	}
	if value.Status == "succeeded" {
		if !jsonObject(value.Candidate) || value.ResultHash == nil || !hashPattern.MatchString(*value.ResultHash) || value.Error != nil {
			return errors.New("successful agent result is incomplete")
		}
		computed, err := CanonicalHash(value.Candidate)
		if err != nil || computed != *value.ResultHash {
			return errors.New("agent result hash mismatch")
		}
		return nil
	}
	if !bytes.Equal(bytes.TrimSpace(value.Candidate), []byte("null")) || value.ResultHash != nil || value.Error == nil || value.Error.Code == "" || value.Error.Summary == "" {
		return errors.New("failed or unknown agent result is incomplete")
	}
	return nil
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

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}
