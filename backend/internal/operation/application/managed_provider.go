package application

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// ErrManagedCapacity means no request was sent because the connection is at its limit.
var ErrManagedCapacity = errors.New("managed provider capacity reached")

// ManagedCredential is worker-only encrypted material for the original account.
// Never serialize this structure into a workflow, API response, or log.
type ManagedCredential struct {
	ProviderID      uuid.UUID
	CredentialID    uuid.UUID
	ConfigVersionID uuid.UUID
	Config          json.RawMessage
	KeyID           string
	Ciphertext      []byte
}

// ManagedProgress is the only provider activity result that enters history.
type ManagedProgress struct {
	State       string `json:"state"`
	TaskID      string `json:"task_id,omitempty"`
	OutputCount int    `json:"output_count,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
}

// ManagedStep requests one explicit submit, read-only poll, or receipt recovery.
type ManagedStep struct {
	OperationID uuid.UUID `json:"operation_id"`
	Action      string    `json:"action"`
}
