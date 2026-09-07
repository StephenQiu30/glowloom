package application

import (
	"context"
	"errors"
	"regexp"
	"time"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

const (
	submitProviderOperation    = "generation.provider.submit"
	reconcileProviderOperation = "generation.provider.reconcile"
	terminalProviderOperation  = "generation.provider.terminal"

	ProviderOutcomeAccepted  = "accepted"
	ProviderOutcomeRunning   = "running"
	ProviderOutcomeSucceeded = "succeeded"
	ProviderOutcomeFailed    = "failed"
	ProviderOutcomeUnknown   = "unknown"

	providerActionNone      = "none"
	providerActionSubmit    = "submit"
	providerActionQuery     = "query"
	providerActionRecover   = "recover"
	providerActionExpire    = "expire"
	providerActionAbandon   = "abandon"
	providerPreflightFailed = "provider.preflight_failed"

	ProviderQueryFailureRetryable             = "retryable"
	ProviderQueryFailureIdentityUnrecoverable = "identity_unrecoverable"
	ProviderSubmitFailureIdentityRecoverable  = "identity_recoverable"
)

var (
	ErrGenerationRequestNotFound     = errors.New("generation request not found")
	ErrProviderJobNotFound           = errors.New("generation Provider job not found")
	ErrProviderCallNotFound          = errors.New("generation Provider call not found")
	ErrProviderResultReceiptNotFound = errors.New("generation Provider result receipt not found")
	providerIdentifierPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,179}$`)
	providerFailurePattern           = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,119}$`)
	providerOutputKeyPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`)
)

type ProviderOutput = domain.ProviderOutput

type ProviderSubmission struct {
	WorkspaceID, ProjectID, ProviderJobID string
	ProviderCallID, CallKey               string
	CallRequestHash                       string
	CandidateIndex, RequestedOutputCount  int
	RequestID, RequestKey, IntentID       string
	ProviderKey, ExternalModelID          string
	ConnectionVersionID                   string
	CredentialVersionID                   string
	BindingID, BindingContentHash         string
	BindingRevision                       int64
	ModelProfileVersionID                 string
	ModelProfileRevision                  int64
	ModelProfileContentHash               string
	PriceQuoteID, PriceQuoteContentHash   string
	PriceQuoteRevision                    int64
	BillingMetric                         string
	EstimatedUnits                        int64
	RemoteRequestID, RemoteJobID          string
	QueryDeadlineAt                       *time.Time
	RemoteExpiresAt                       *time.Time
	TargetHash                            string
	Target                                domain.GenerationTarget
}

type ProviderOutcome struct {
	Status                       string
	RemoteRequestID, RemoteJobID string
	ProviderEventID, FailureCode string
	Output                       *ProviderOutput
	ProviderUsageObservation     domain.ProviderUsageObservation
	OccurredAt                   time.Time
	QueryDeadlineAt              time.Time
	RemoteExpiresAt              time.Time
}

type ProviderGateway interface {
	Preflight(context.Context, ProviderSubmission) error
	Submit(context.Context, ProviderSubmission) (ProviderOutcome, error)
	Query(context.Context, ProviderSubmission) (ProviderOutcome, error)
}

type ProviderLocalFailure interface {
	ProviderFailureCode() string
}

type ProviderQueryFailure interface {
	ProviderQueryFailureKind() string
}

// ProviderSubmitFailure is implemented only when a Submit error still carries
// an official, queryable remote task identity. Generic Submit errors remain
// outcome-unknown because they cannot prove that the returned identity is safe
// to reconcile.
type ProviderSubmitFailure interface {
	ProviderSubmitFailureKind() string
}

type CostProviderOwner interface {
	GetReservation(context.Context, costapp.Actor, string) (costapp.ReservationView, error)
	SettleReservation(context.Context, costapp.Actor, costapp.SettleReservationCommand) (costapp.ReservationResult, error)
	ReleaseReservation(context.Context, costapp.Actor, costapp.ReleaseReservationCommand) (costapp.ReservationResult, error)
}

type QuotaProviderOwner interface {
	GetReservation(context.Context, quotaapp.Actor, string) (quotadomain.Reservation, error)
	Consume(context.Context, quotaapp.Actor, quotaapp.TransitionCommand) (quotaapp.ReservationResult, error)
	Release(context.Context, quotaapp.Actor, quotaapp.TransitionCommand) (quotaapp.ReservationResult, error)
}

type ProviderRepository interface {
	PreparationRepository
	LockProviderWorkspace(context.Context, string) error
	LatestProviderConnectionForUpdate(context.Context, string, string) (domain.ProviderConnectionVersion, error)
	LatestProviderModelProfileForUpdate(context.Context, string, string) (domain.ProviderModelProfileVersion, error)
	FindProjectProviderBinding(context.Context, string) (domain.ProjectProviderBindingVersion, error)
	FindProviderModelProfile(context.Context, string) (domain.ProviderModelProfileVersion, error)
	FindRequestByIntent(context.Context, string) (domain.GenerationRequest, error)
	FindGenerationRequest(context.Context, string) (domain.GenerationRequest, error)
	EnsureRequestJobAndCalls(
		context.Context,
		domain.GenerationRequest,
		domain.ProviderJob,
		[]domain.ProviderCall,
	) (domain.GenerationRequest, domain.ProviderJob, []domain.ProviderCall, error)
	FindProviderJobByIntent(context.Context, string) (domain.ProviderJob, error)
	GetIntentForProviderJobUpdate(context.Context, string) (domain.Intent, error)
	GetProviderJobForUpdate(context.Context, string) (domain.ProviderJob, error)
	UpdateProviderJob(context.Context, domain.ProviderJob, int64) (domain.ProviderJob, error)
	ListProviderCalls(context.Context, string) ([]domain.ProviderCall, error)
	GetProviderCallForUpdate(context.Context, string) (domain.ProviderCall, error)
	UpdateProviderCall(context.Context, domain.ProviderCall, int64) (domain.ProviderCall, error)
	ListProviderResultReceipts(context.Context, string) ([]domain.ProviderResultReceipt, error)
	FindProviderResultReceiptByCall(context.Context, string) (domain.ProviderResultReceipt, error)
	EnsureProviderResultReceipt(context.Context, domain.ProviderResultReceipt) (domain.ProviderResultReceipt, error)
}

type ProviderTransactionManager interface {
	WithinProviderTransaction(
		context.Context,
		func(ProviderRepository, CostProviderOwner, QuotaProviderOwner) error,
	) error
}

type ProviderProjectScope struct {
	WorkspaceID, ProjectID string
}

type SubmitImageRequestCommand struct {
	IntentID, IdempotencyKey string
}

type ReconcileProviderJobCommand struct {
	ProviderJobID, IdempotencyKey string
}

type ProviderExecutionResult struct {
	Intent   domain.Intent
	Target   domain.GenerationTarget
	Request  domain.GenerationRequest
	Job      domain.ProviderJob
	Calls    []domain.ProviderCall
	Receipts []domain.ProviderResultReceipt
	Receipt  platformcommand.Receipt
}
