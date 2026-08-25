package application

import (
	"context"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type SignalRepository interface {
	PrepareSignal(context.Context, domain.SignalPreparation) (domain.SignalPreparation, error)
	BeginSignalAttempt(context.Context, string, time.Time) (domain.SignalPreparation, error)
	FinalizeSignalAttempt(context.Context, domain.SignalIntent, domain.SignalReceipt, int) error
}

type WorkflowSignaler interface {
	Signal(context.Context, domain.SignalRequest) (domain.SignalObservation, error)
}

type SignalConfig struct {
	Now   func() time.Time
	NewID func() string
}

type SignalService struct {
	repository SignalRepository
	signaler   WorkflowSignaler
	config     SignalConfig
}

type SignalHumanGateCommand struct {
	WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID         string
	SubjectRevision                       int
	Decision, IdempotencyKey              string
}

func NewSignalService(repository SignalRepository, signaler WorkflowSignaler, config SignalConfig) *SignalService {
	return &SignalService{repository: repository, signaler: signaler, config: config}
}

func (service *SignalService) SignalHumanGate(
	ctx context.Context,
	actor Actor,
	command SignalHumanGateCommand,
) (domain.SignalIntent, error) {
	command = normalizeSignalCommand(command)
	actor.UserID = strings.TrimSpace(actor.UserID)
	if service == nil || service.repository == nil || service.signaler == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.UserID == "" || !validSignalCommand(command) {
		return domain.SignalIntent{}, invalid("Invalid human gate signal input")
	}
	commandInputHash, err := platformcommand.InputHash(struct {
		WorkspaceID, WorkflowRunID, NodeRunID string
		HumanTaskID, ReviewDecisionID         string
		SubjectRevision                       int
		Decision                              string
	}{
		WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
		SubjectRevision: command.SubjectRevision, Decision: command.Decision,
	})
	if err != nil {
		return domain.SignalIntent{}, err
	}
	now := service.config.Now().UTC()
	intentID := stableID("workflow-signal-intent", command.WorkspaceID, command.WorkflowRunID, command.ReviewDecisionID)
	applyReceiptID := stableID("workflow-human-gate-apply", command.WorkspaceID, command.ReviewDecisionID)
	signalID := stableID("workflow-human-gate-signal", intentID)
	desired := domain.SignalPreparation{
		ApplyReceipt: domain.HumanGateApplyReceipt{
			ID: applyReceiptID, WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
			NodeRunID: command.NodeRunID, HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
			SubjectRevision: command.SubjectRevision, Decision: command.Decision,
			CreatedBy: actor.UserID, CreatedAt: now,
		},
		Intent: domain.SignalIntent{
			ID: intentID, WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
			NodeRunID: command.NodeRunID, HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
			IdempotencyKey: command.IdempotencyKey, CommandInputHash: commandInputHash,
			SignalID: signalID, Decision: command.Decision, SubjectRevision: command.SubjectRevision,
			Status: "pending", Revision: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		},
	}
	prepared, err := service.repository.PrepareSignal(ctx, desired)
	if err != nil {
		return domain.SignalIntent{}, normalizeError(err)
	}
	request, err := NewSignalRequest(prepared.Intent)
	if err != nil {
		return domain.SignalIntent{}, err
	}
	if prepared.Intent.CommandInputHash != commandInputHash || prepared.Intent.InputHash != request.InputHash ||
		prepared.Intent.ReviewDecisionID != command.ReviewDecisionID {
		return domain.SignalIntent{}, conflict("Idempotency key was already used with different signal input")
	}
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		return prepared.Intent, nil
	}
	prepared, err = service.repository.BeginSignalAttempt(ctx, prepared.Intent.ID, service.config.Now().UTC())
	if err != nil {
		return domain.SignalIntent{}, normalizeError(err)
	}
	if prepared.Intent.Status == "completed" || prepared.Intent.Status == "conflict" {
		return prepared.Intent, nil
	}
	observation, signalErr := service.signaler.Signal(ctx, request)
	updated, receipt := finalizeSignal(prepared.Intent, observation, signalErr, service.config.NewID(), service.config.Now().UTC())
	if err = service.repository.FinalizeSignalAttempt(ctx, updated, receipt, prepared.Intent.Revision); err != nil {
		return domain.SignalIntent{}, normalizeError(err)
	}
	return updated, nil
}

func NewSignalRequest(intent domain.SignalIntent) (domain.SignalRequest, error) {
	request := domain.SignalRequest{
		TemporalWorkflowID: intent.TemporalWorkflowID, SignalID: intent.SignalID, SignalIntentID: intent.ID,
		WorkflowRunID: intent.WorkflowRunID, NodeRunID: intent.NodeRunID, Decision: strings.ToUpper(intent.Decision),
	}
	inputHash, err := platformcommand.InputHash(request)
	if err != nil {
		return domain.SignalRequest{}, err
	}
	request.InputHash = inputHash
	return request, nil
}

func finalizeSignal(
	intent domain.SignalIntent,
	observation domain.SignalObservation,
	signalErr error,
	receiptID string,
	now time.Time,
) (domain.SignalIntent, domain.SignalReceipt) {
	outcome := observation.Outcome
	var observed *string
	if len(observation.ObservedInputHash) == 64 {
		value := observation.ObservedInputHash
		observed = &value
	}
	switch {
	case signalErr != nil || outcome == domain.SignalOutcomeUnknown:
		outcome, intent.Status = domain.SignalOutcomeUnknown, "unknown"
	case (outcome == domain.SignalOutcomeSignaled || outcome == domain.SignalOutcomeAlreadyApplied) &&
		observation.ObservedInputHash == intent.InputHash:
		intent.Status = "completed"
	default:
		outcome, intent.Status = domain.SignalOutcomeConflict, "conflict"
	}
	intent.Revision++
	intent.UpdatedAt = now
	receipt := domain.SignalReceipt{
		ID: receiptID, WorkspaceID: intent.WorkspaceID, SignalIntentID: intent.ID, WorkflowRunID: intent.WorkflowRunID,
		AttemptNo: intent.AttemptNo, Outcome: outcome, SignalID: intent.SignalID,
		ExpectedInputHash: intent.InputHash, ObservedInputHash: observed, CreatedAt: now,
	}
	return intent, receipt
}

func normalizeSignalCommand(command SignalHumanGateCommand) SignalHumanGateCommand {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.WorkflowRunID = strings.TrimSpace(command.WorkflowRunID)
	command.NodeRunID = strings.TrimSpace(command.NodeRunID)
	command.HumanTaskID = strings.TrimSpace(command.HumanTaskID)
	command.ReviewDecisionID = strings.TrimSpace(command.ReviewDecisionID)
	command.Decision = strings.ToLower(strings.TrimSpace(command.Decision))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	return command
}

func validSignalCommand(command SignalHumanGateCommand) bool {
	if command.WorkspaceID == "" || command.WorkflowRunID == "" || command.NodeRunID == "" || command.HumanTaskID == "" ||
		command.ReviewDecisionID == "" || command.SubjectRevision < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return false
	}
	switch command.Decision {
	case "approved", "rejected", "changes_requested", "selected":
		return true
	default:
		return false
	}
}
