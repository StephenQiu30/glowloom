package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// The owned work budget leaves room for durable finalization before the activity
// deadline. Native HTTP termination must fit even when caller cancellation is detached.
const (
	TranscriptionPreparationTimeout  = 5 * time.Minute
	TranscriptionInferenceTimeout    = 20 * time.Minute
	TranscriptionFinalizationTimeout = time.Minute
	TranscriptionActivityTimeout     = 30 * time.Minute
)

var (
	// ErrInvalidTranscription rejects unsupported input or local inference results.
	ErrInvalidTranscription = errors.New("invalid media transcription")
	// ErrNoSpeech preserves the absence of recognized speech instead of fabricating cues.
	ErrNoSpeech = errors.New("no recognized speech")
	// ErrInferenceUncertain means no terminal HTTP response proves local inference ended.
	ErrInferenceUncertain = errors.New("local inference completion unknown")
)

// TranscriptionSourceReader freezes a saved audio/video node under its owner lock.
type TranscriptionSourceReader interface {
	FreezeTranscriptionSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error)
}

// AudioPreprocessor creates a bounded full-duration 16 kHz mono PCM WAV.
type AudioPreprocessor interface {
	Prepare(context.Context, *mediaapp.Downloaded, int64) (*PreparedAudio, error)
}

// PreparedAudio owns a complete PCM file and the duration derived from its samples.
type PreparedAudio struct {
	File       *mediaapp.Downloaded
	DurationMS int64
}

// Transcriber returns only actual, validated native local inference cues.
type Transcriber interface {
	Transcribe(context.Context, *mediaapp.Downloaded, string, int64) (domain.Transcript, error)
}

// TranscriptionCreateInput contains saved identity and a native language code.
type TranscriptionCreateInput struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	NodeID   uuid.UUID `json:"node_id"`
	Revision int64     `json:"revision"`
	Language string    `json:"language,omitempty" default:"auto"`
}

// TranscriptionResult binds a real draft to the exact durable job revision.
type TranscriptionResult struct {
	JobID               uuid.UUID         `json:"job_id"`
	Revision            int64             `json:"revision"`
	SHA256              string            `json:"sha256"`
	Draft               domain.Transcript `json:"draft"`
	SRT                 string            `json:"srt"`
	SourceAssetID       uuid.UUID         `json:"source_asset_id"`
	SourceAssetRevision int64             `json:"source_asset_revision"`
	SourceSHA256        string            `json:"source_sha256"`
}

// TranscriptionPage restores safe facts with the project-bound HTTP cursor.
type TranscriptionPage struct {
	Items      []domain.TranscriptionJob `json:"items"`
	NextCursor *string                   `json:"next_cursor" extensions:"x-nullable"`
}

// TranscriptionStore owns durable speech commands independently of ExportJob.
type TranscriptionStore interface {
	Create(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, TranscriptionCreateInput) (domain.TranscriptionJob, error)
	Get(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.TranscriptionJob, error)
	List(context.Context, identityapp.Principal, ListInput) ([]domain.TranscriptionJob, error)
	Result(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (TranscriptionResult, error)
	Control(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64, string) (domain.TranscriptionJob, error)
}

// TranscriptionWorkID fences one attempt and one actual local activity owner.
type TranscriptionWorkID struct {
	JobID       uuid.UUID `json:"job_id"`
	Attempt     int       `json:"attempt"`
	ExecutionID uuid.UUID `json:"-"`
}

// TranscriptionWork contains immutable originals and the initiating actor.
type TranscriptionWork struct {
	Job            domain.TranscriptionJob
	Actor          identityapp.Principal
	Frozen         domain.FrozenTranscription
	InferenceState string
}

// TranscriptionWorkerStore never infers remote cessation from a client timeout.
type TranscriptionWorkerStore interface {
	Claim(context.Context, TranscriptionWorkID) (TranscriptionWork, error)
	Progress(context.Context, TranscriptionWorkID, int, string) error
	StartInference(context.Context, TranscriptionWorkID) error
	EndInference(context.Context, TranscriptionWorkID) error
	Complete(context.Context, TranscriptionWorkID, domain.Transcript) (domain.TranscriptionJob, error)
	Finish(context.Context, TranscriptionWorkID, bool, string) error
	Release(context.Context, TranscriptionWorkID) error
	FailWorkflow(context.Context, TranscriptionWorkID) error
}

// TranscriptionDelivery is proved against the committed speech command receipt.
type TranscriptionDelivery struct {
	TranscriptionWorkID
	EventID   uuid.UUID `json:"event_id"`
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Action    string    `json:"action"`
}
