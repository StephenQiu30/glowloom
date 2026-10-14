package application

import (
	"context"
	"errors"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/google/uuid"
)

var (
	ErrSubmissionUnresolved = errors.New("submission_unresolved")
	ErrSubmissionEpoch      = errors.New("submission_epoch_conflict")
	ErrSubmissionAccepted   = errors.New("submission_already_accepted")
)

type EditorSnapshots interface {
	AuthorizeOwner(context.Context, identityapp.Principal, uuid.UUID, promptdomain.InputSource) error
	ReadInput(context.Context, identityapp.Principal, uuid.UUID, promptdomain.InputSource, bool) (promptdomain.Draft, error)
}
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
type SubmissionCurrent struct {
	Epoch      int64       `json:"epoch"`
	Submission *Submission `json:"submission,omitempty"`
}
type SubmissionResolution struct {
	ProjectID     uuid.UUID
	ID            uuid.UUID
	ExpectedEpoch int64
	Action        string
	Key           uuid.UUID
}
type EditorSubmissions interface {
	CurrentSubmission(context.Context, identityapp.Principal, uuid.UUID, promptdomain.InputSource) (SubmissionCurrent, error)
	ResolveSubmission(context.Context, identityapp.Principal, SubmissionResolution) (SubmissionCurrent, error)
}

type EditorQuote struct {
	QuoteMicros     int64     `json:"quote_micros"`
	AvailableMicros int64     `json:"available_micros"`
	FinalPrompt     string    `json:"final_prompt"`
	ExpiresAt       time.Time `json:"expires_at"`
	Confirmable     bool      `json:"confirmable"`
}
