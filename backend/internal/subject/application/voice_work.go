package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// VoiceOwner keeps target authority separate from the actor's output library.
type VoiceOwner struct {
	ScopeID, PhysicalScopeID, PersonalScopeID, PersonalOrgID uuid.UUID
}

// VoiceAccess is bound to the request or worker's exact SQL transaction.
type VoiceAccess interface {
	Request(context.Context, identity.Principal, bool) (VoiceOwner, error)
	Worker(context.Context, VoiceWork) (VoiceOwner, error)
}

// VoiceRelease contains owner-proven immutable execution and cost identities.
type VoiceRelease struct {
	VoiceModelOption
	ProviderID, ConfigVersionID, CredentialID, PriceVersionID uuid.UUID
	ResourceID, Moderation                                    string
	UpstreamCost                                              json.RawMessage
}

// VoiceCredential stays inside the worker and is never a workflow argument.
type VoiceCredential struct {
	Config                   json.RawMessage `json:"-"`
	ProviderID, CredentialID uuid.UUID
	KeyID                    string
	Ciphertext               []byte `json:"-"`
}

// Format refuses secret-bearing generic logging.
func (VoiceCredential) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[sealed speech credential]")
}

// VoiceCatalog is a consumer-owned port for actual active releases and keys.
type VoiceCatalog interface {
	List(context.Context) ([]VoiceRelease, error)
	Select(context.Context, uuid.UUID, string) (VoiceRelease, error)
	Credential(context.Context, VoiceWork) (VoiceCredential, error)
}

// VoiceStorageIntent is the exact existing object ledger admission, never a URL.
type VoiceStorageIntent struct {
	ID                                       uuid.UUID
	ProfileID, Bucket, ConfigRevision, State string
}

// VoiceStorage admits a route once and reloads that exact original on recovery.
type VoiceStorage interface {
	Ready(context.Context) (bool, error)
	Pin(context.Context, uuid.UUID, uuid.UUID) (VoiceStorageIntent, error)
	Read(context.Context, uuid.UUID, uuid.UUID) (VoiceStorageIntent, error)
}

// VoiceWork is internal frozen admission. It does not contain plaintext keys.
type VoiceWork struct {
	Receipt                                                   *VoiceReceipt
	Job                                                       VoiceJob
	Owner                                                     VoiceOwner
	ProviderID, ConfigVersionID, CredentialID, PriceVersionID uuid.UUID
	AssetID, EventID, ExecutionID                             uuid.UUID
	ResourceID, Moderation, RequestSHA256                     string
	SpeechRate                                                int
	UpstreamCost                                              json.RawMessage
	Storage                                                   VoiceStorageIntent
	DispatchStartedAt                                         *time.Time
}

// VoiceWorkID contains only durable identities. RecoveryOnly forbids submission.
type VoiceWorkID struct {
	JobID        uuid.UUID `json:"job_id"`
	RecoveryOnly bool      `json:"recovery_only"`
}

// VoiceReceipt is a private immutable success manifest for exact original bytes.
type VoiceReceipt struct {
	SchemaVersion      int             `json:"schema_version"`
	JobID              uuid.UUID       `json:"job_id"`
	ActorID            uuid.UUID       `json:"actor_id"`
	OwnerScopeID       uuid.UUID       `json:"owner_scope_id"`
	PersonalOrgID      uuid.UUID       `json:"personal_org_id"`
	AssetID            uuid.UUID       `json:"asset_id"`
	ReleaseID          uuid.UUID       `json:"release_id"`
	ModelVersionID     uuid.UUID       `json:"model_version_id"`
	ProviderRequestID  uuid.UUID       `json:"provider_request_id"`
	RequestSHA256      string          `json:"request_sha256"`
	ProviderLogID      string          `json:"provider_log_id,omitempty"`
	AudioLocationID    uuid.UUID       `json:"audio_location_id"`
	ManifestLocationID uuid.UUID       `json:"manifest_location_id"`
	AudioKey           string          `json:"audio_key"`
	AudioSHA256        string          `json:"audio_sha256"`
	AudioByteSize      int64           `json:"audio_byte_size"`
	MIMEType           string          `json:"mime_type"`
	TerminalCode       int             `json:"terminal_code"`
	Usage              json.RawMessage `json:"usage,omitempty"`
}

// VoiceArtifacts persists before DB acknowledgement and can recover without a send.
type VoiceArtifacts interface {
	Save(context.Context, VoiceWork, VoiceSynthesisResult) (VoiceReceipt, error)
	Read(context.Context, VoiceWork) (VoiceReceipt, bool, error)
}

// VoiceWorkerStore holds the durable at-most-once submission fence.
type VoiceWorkerStore interface {
	Work(context.Context, uuid.UUID) (VoiceWork, error)
	Claim(context.Context, uuid.UUID, uuid.UUID) (VoiceWork, bool, error)
	Credential(context.Context, VoiceWork) (VoiceCredential, error)
	RecordReceipt(context.Context, VoiceWork, VoiceReceipt) (VoiceWork, error)
	MarkUnknown(context.Context, VoiceWork, string) (VoiceJob, error)
	MarkFailed(context.Context, VoiceWork, string) (VoiceJob, error)
}

// VoiceOutputPublisher probes and atomically accepts media and marks the same job
// succeeded. It may safely repeat local ingest from an immutable success receipt.
type VoiceOutputPublisher interface {
	Publish(context.Context, VoiceWork, VoiceReceipt) (VoiceJob, error)
}

// VoiceUnsealer is the existing worker-only authenticated envelope mechanism.
type VoiceUnsealer interface {
	Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error)
}
