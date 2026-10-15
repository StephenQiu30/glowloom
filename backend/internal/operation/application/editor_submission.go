package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

var (
	// ErrSubmissionUnresolved requires recovery before starting another intent.
	ErrSubmissionUnresolved = errors.New("submission_unresolved")
	// ErrSubmissionEpoch rejects stale or competing slot generations.
	ErrSubmissionEpoch = errors.New("submission_epoch_conflict")
	// ErrSubmissionAccepted prevents discarding a reserved execution.
	ErrSubmissionAccepted = errors.New("submission_already_accepted")
)

// EditorSnapshots reads authorized, persisted editor intent under transaction locks.
type EditorSnapshots interface {
	AuthorizeOwner(context.Context, identityapp.Principal, uuid.UUID, promptdomain.InputSource) error
	ReadInput(context.Context, identityapp.Principal, uuid.UUID, promptdomain.InputSource, bool) (promptdomain.Draft, error)
}

// Submission is the durable, private generation intent and canonical confirmation identity.
type Submission struct {
	ID          uuid.UUID                `json:"id"`
	Epoch       int64                    `json:"epoch"`
	OperationID uuid.UUID                `json:"operation_id"`
	ConfirmKey  uuid.UUID                `json:"confirm_key"`
	InputSource promptdomain.InputSource `json:"input_source"`
	Draft       promptdomain.Draft       `json:"draft"`
	Quote       *EditorQuote             `json:"quote,omitempty"`
	State       string                   `json:"state"`
}

// SubmissionCurrent includes the epoch even when no active submission remains.
type SubmissionCurrent struct {
	Epoch      int64       `json:"epoch"`
	Submission *Submission `json:"submission,omitempty"`
}

// SubmissionResolution describes one idempotent user decision about a slot.
type SubmissionResolution struct {
	ProjectID     uuid.UUID
	ID            uuid.UUID
	ExpectedEpoch int64
	Action        string
	Key           uuid.UUID
}

// EditorSubmissions owns discovery and resolution independent of local storage.
type EditorSubmissions interface {
	CurrentSubmission(context.Context, identityapp.Principal, uuid.UUID, promptdomain.InputSource) (SubmissionCurrent, error)
	ResolveSubmission(context.Context, identityapp.Principal, SubmissionResolution) (SubmissionCurrent, error)
}

// EditorQuote preserves the user-visible estimate and compiler mapping.
type EditorQuote struct {
	CompilerVersion  string                          `json:"compiler_version,omitempty"`
	ReferenceMapping []promptdomain.ReferenceMapping `json:"reference_mapping,omitempty"`
	QuoteMicros      int64                           `json:"quote_micros"`
	AvailableMicros  int64                           `json:"available_micros"`
	FinalPrompt      string                          `json:"final_prompt"`
	ExpiresAt        time.Time                       `json:"expires_at"`
	Confirmable      bool                            `json:"confirmable"`
}
