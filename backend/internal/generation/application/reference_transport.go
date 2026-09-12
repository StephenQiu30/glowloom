package application

import (
	"context"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

// ReferenceImageSubmission is reconstructed from the frozen execution manifest.
// Its caller must verify scope, provider versions and current execution fences.
// Neither this value nor a valid dispatch token proves a committed send right.
type ReferenceImageSubmission struct {
	WorkspaceID string                       `json:"workspace_id"`
	ProjectID   string                       `json:"project_id"`
	Call        domain.ReferenceProviderCall `json:"call"`
	Request     ReferenceImageRequest        `json:"request"`
	Slot        domain.ReferenceOutputSlot   `json:"slot"`
}

const (
	ReferenceImageStaged         = "staged"
	ReferenceImageOutputRejected = "output_rejected"
	ReferenceImageOutcomeUnknown = "outcome_unknown"
)

// ReferenceImageObservation contains no raw response, prompt, credential or URL.
// It is transport evidence, not a persisted receipt or an accepted AssetVersion.
type ReferenceImageObservation struct {
	CallKey         string                          `json:"call_key"`
	SubmissionToken string                          `json:"submission_token"`
	ObservedAt      time.Time                       `json:"observed_at"`
	Status          string                          `json:"status"`
	ReasonCode      string                          `json:"reason_code"`
	Output          *domain.ProviderOutput          `json:"output,omitempty"`
	Usage           domain.ProviderUsageObservation `json:"usage"`
}

// ReferenceImageRuntime borrows invocation-scoped credentials. The application
// must Preflight before claiming, then Submit only on its own successful claim
// commit, within the same invocation. Never replay Submit from a stored token.
// Errors mean no HTTP call was attempted; post-send uncertainty is an observation.
type ReferenceImageRuntime interface {
	Preflight(context.Context, ReferenceImageSubmission) error
	Submit(context.Context, ReferenceImageSubmission, domain.ReferenceCallDispatch) (ReferenceImageObservation, error)
}

type ReferenceImageExecutableFactory interface {
	ReferenceImageCompiler
	NewReferenceImageRuntime(ProviderRuntimeConfig) (ReferenceImageRuntime, error)
}
