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
	ModelID         string
	EndpointTag     string
	ProviderID      uuid.UUID
	CredentialID    uuid.UUID
	ConfigVersionID uuid.UUID
	Config          json.RawMessage
	KeyID           string
	Ciphertext      []byte
}

// ManagedProgressState classifies the current managed generation outcome.
type ManagedProgressState string

// Managed progress states are the bounded workflow contract for one managed provider call.
const (
	ManagedProgressWaiting      ManagedProgressState = "waiting"
	ManagedProgressNotSubmitted ManagedProgressState = "not_submitted"
	ManagedProgressUnknown      ManagedProgressState = "unknown"
	ManagedProgressRejected     ManagedProgressState = "rejected"
	ManagedProgressAccepted     ManagedProgressState = "accepted"
	ManagedProgressSucceeded    ManagedProgressState = "succeeded"
	ManagedProgressPending      ManagedProgressState = "pending"
	ManagedProgressRunning      ManagedProgressState = "running"
	ManagedProgressFailed       ManagedProgressState = "failed"
)

// ManagedProgress contains the classified outcome and durable provider progress.
type ManagedProgress struct {
	HTTPStatus    int                  `json:"http_status,omitempty"`
	ProviderLogID string               `json:"provider_log_id,omitempty"`
	State         ManagedProgressState `json:"state"`
	TaskID        string               `json:"task_id,omitempty"`
	OutputCount   int                  `json:"output_count,omitempty"`
	ErrorCode     string               `json:"error_code,omitempty"`
}

// ManagedStep requests one explicit submit, read-only poll, or receipt recovery.
type ManagedStep struct {
	OperationID uuid.UUID `json:"operation_id"`
	Action      string    `json:"action"`
}
