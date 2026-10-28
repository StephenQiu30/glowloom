package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrInvalidVoiceJob rejects malformed sample-generation intent.
	ErrInvalidVoiceJob = errors.New("invalid subject voice job")
	// ErrVoiceUnavailable rejects unconfigured or disabled speech execution.
	ErrVoiceUnavailable = errors.New("subject voice generation unavailable")
	// ErrVoiceJobNotFound hides jobs outside their exact actor and target scope.
	ErrVoiceJobNotFound = errors.New("subject voice job not found")
	// ErrVoiceJobKeyConflict preserves one exact permanent generation intent.
	ErrVoiceJobKeyConflict = errors.New("subject voice job key conflict")
)

// VoiceOption is an explicit configured speaker, never a generated preview.
type VoiceOption struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// VoiceModelOption describes a real enabled immutable speech release.
type VoiceModelOption struct {
	ReleaseID       uuid.UUID     `json:"release_id"`
	ModelVersionID  uuid.UUID     `json:"model_version_id"`
	ModelKey        string        `json:"model_key"`
	DisplayName     string        `json:"display_name"`
	Voices          []VoiceOption `json:"voices"`
	DefaultVoiceKey string        `json:"default_voice_key,omitempty"`
}

// VoiceOptions binds configuration availability to the current actor and scope.
type VoiceOptions struct {
	CurrentActorID uuid.UUID          `json:"current_actor_id"`
	CurrentScopeID uuid.UUID          `json:"current_scope_id"`
	Available      bool               `json:"available"`
	Reason         string             `json:"reason,omitempty"`
	Models         []VoiceModelOption `json:"models"`
}

// CreateVoiceJobInput is explicit TTS intent; it cannot carry keys or asset facts.
type CreateVoiceJobInput struct {
	DraftIntentID uuid.UUID `json:"draft_intent_id"`
	ReleaseID     uuid.UUID `json:"release_id"`
	VoiceKey      string    `json:"voice_key"`
	Name          string    `json:"name"`
	Text          string    `json:"text"`
	SpeechRate    *int      `json:"speech_rate,omitempty"`
}

// VoiceJobResult is a formal ready personal asset for the existing Subject save.
// No binding is invented: adopting it creates the formal immutable binding.
type VoiceJobResult struct {
	AssetID       uuid.UUID `json:"asset_id"`
	AssetRevision int64     `json:"asset_revision"`
	Kind          string    `json:"kind"`
	MIMEType      string    `json:"mime_type"`
	FileName      string    `json:"file_name"`
	LibraryItemID uuid.UUID `json:"library_item_id"`
	LibrarySource string    `json:"library_source"`
}

// VoiceJob retains exact draft identity across polling and permanent-key replay.
// Unknown is an uncertain paid execution; it never requests an automatic retry.
type VoiceJob struct {
	ID             uuid.UUID       `json:"id"`
	CurrentActorID uuid.UUID       `json:"current_actor_id"`
	CurrentScopeID uuid.UUID       `json:"current_scope_id"`
	DraftIntentID  uuid.UUID       `json:"draft_intent_id"`
	Revision       int64           `json:"revision"`
	Status         string          `json:"status"` // queued/dispatching/ingesting/succeeded/failed/unknown
	Name           string          `json:"name"`
	Text           string          `json:"text"`
	ReleaseID      uuid.UUID       `json:"release_id"`
	ModelVersionID uuid.UUID       `json:"model_version_id"`
	VoiceKey       string          `json:"voice_key"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ErrorCode      string          `json:"error_code,omitempty"`
	Result         *VoiceJobResult `json:"result,omitempty"`
}

// VoiceRecovery looks up only the caller's explicit permanent command key.
// Unrecorded does not prove that an in-flight HTTP request has ceased.
type VoiceRecovery struct {
	State string    `json:"state"`
	Job   *VoiceJob `json:"job,omitempty"`
}

// VoiceJobs owns current authority, immutable admission and exact recovery.
type VoiceJobs interface {
	Options(context.Context, identity.Principal) (VoiceOptions, error)
	Create(context.Context, identity.Principal, uuid.UUID, CreateVoiceJobInput) (VoiceJob, error)
	Get(context.Context, identity.Principal, uuid.UUID) (VoiceJob, error)
	Recover(context.Context, identity.Principal, uuid.UUID) (VoiceRecovery, error)
}
