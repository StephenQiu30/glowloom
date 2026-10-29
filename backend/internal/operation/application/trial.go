package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrInvalidTrial rejects malformed administrator experiment input.
var ErrInvalidTrial = errors.New("invalid development trial")

// TrialRequest is a local administrator experiment, independent of billing and releases.
type TrialRequest struct {
	RequestID  uuid.UUID           `json:"request_id"`
	Source     string              `json:"source"`
	Kind       string              `json:"kind"`
	ProviderID uuid.UUID           `json:"provider_id"`
	Model      string              `json:"model"`
	Prompt     string              `json:"prompt"`
	Mode       string              `json:"mode"`
	Params     map[string]any      `json:"params"`
	References []ProviderReference `json:"references"`
}

// Validate bounds the supported source, capability and input shape.
func (r TrialRequest) Validate() error {
	if r.RequestID == uuid.Nil || (r.Source != "codex" && r.Source != "cloud") {
		return ErrInvalidTrial
	}
	if r.Kind == "probe" && r.Source == "codex" && r.Model == "" && r.Prompt == "" && r.ProviderID == uuid.Nil && len(r.Params) == 0 && len(r.References) == 0 && r.Mode == "" {
		return nil
	}
	if r.Kind != "chat" && r.Kind != "image" && r.Kind != "video" && r.Kind != "audio" {
		return ErrInvalidTrial
	}
	if (strings.TrimSpace(r.Prompt) == "" && (r.Kind != "video" || (r.Mode != "avatar" && r.Mode != "motion"))) || len(r.Prompt) > 32*1024 || strings.TrimSpace(r.Model) == "" || len(r.Model) > 200 {
		return ErrInvalidTrial
	}
	encodedPrompt, _ := json.Marshal(r.Prompt)
	if len(encodedPrompt) > 32*1024 {
		return ErrInvalidTrial
	}
	raw, err := json.Marshal(r.Params)
	if err != nil || len(raw) > 4096 || len(r.References) > 4 {
		return ErrInvalidTrial
	}
	if r.Source == "codex" && (r.Kind == "video" || r.Kind == "audio" || r.ProviderID != uuid.Nil || len(r.Params) != 0 || len(r.References) != 0) {
		return ErrInvalidTrial
	}
	if r.Source == "cloud" && (r.ProviderID == uuid.Nil || r.Kind == "chat") {
		return ErrInvalidTrial
	}
	return nil
}

// Fingerprint binds a request ID to its exact original input.
func (r TrialRequest) Fingerprint() string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TrialInput contains server-owned execution and recovery bindings.
type TrialInput struct {
	Request             TrialRequest
	Actor               identityapp.Principal
	ThreadID            string
	ConfigVersionID     uuid.UUID
	CredentialID        uuid.UUID
	TaskID              string
	Action              string
	OriginalFingerprint string
}

// TrialArtifact identifies a verified immutable private result.
type TrialArtifact struct {
	StorageID string `json:"storage_id"`
	Sequence  int    `json:"sequence"`
	MIME      string `json:"mime"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// TrialProgress records one execution result without storing media bytes.
type TrialProgress struct {
	State           string          `json:"state"`
	ErrorCode       string          `json:"error_code,omitempty"`
	Text            string          `json:"text,omitempty"`
	Models          []string        `json:"models,omitempty"`
	Authenticated   bool            `json:"authenticated"`
	ImageGeneration bool            `json:"image_generation"`
	Artifacts       []TrialArtifact `json:"artifacts,omitempty"`
	ThreadID        string          `json:"thread_id,omitempty"`
	TaskID          string          `json:"task_id,omitempty"`
	ConfigVersionID uuid.UUID       `json:"config_version_id"`
	CredentialID    uuid.UUID       `json:"credential_id"`
}

// TrialTurn binds a prompt and result to one idempotent request.
type TrialTurn struct {
	RequestID uuid.UUID `json:"request_id"`
	Prompt    string    `json:"prompt"`
	TrialProgress
}

// TrialState is the durable experiment state queried by its owner.
type TrialState struct {
	Source      string      `json:"source"`
	ProviderID  uuid.UUID   `json:"provider_id"`
	Fingerprint string      `json:"fingerprint"`
	Kind        string      `json:"kind"`
	Model       string      `json:"model"`
	State       string      `json:"state"`
	Turns       []TrialTurn `json:"turns"`
}

// TrialMessage admits a subsequent turn without accepting an upstream thread ID.
type TrialMessage struct {
	RequestID uuid.UUID `json:"request_id"`
	Prompt    string    `json:"prompt"`
}

// TrialWorkflowID isolates requests by organization and administrator.
func TrialWorkflowID(actor identityapp.Principal, id uuid.UUID) string {
	return "ai-trial/" + actor.OrgID.String() + "/" + actor.ID.String() + "/" + id.String()
}

// TrialObjectKey selects the original experiment result deterministically.
func TrialObjectKey(input TrialInput, seq int) string {
	return TrialWorkflowID(input.Actor, input.Request.RequestID) + "/result/" + strconv.Itoa(seq)
}

// TrialStorageID prevents reading a result from a changed storage target.
func TrialStorageID(endpoint, bucket string) string {
	h := sha256.Sum256([]byte(endpoint + "\n" + bucket))
	return hex.EncodeToString(h[:])
}
