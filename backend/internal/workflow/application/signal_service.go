package application

import (
	"context"
	"errors"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type SignalRepository interface {
	FindSignalPreparation(context.Context, string, string) (domain.SignalPreparation, bool, error)
	ResolveHumanGateOwnerApplication(context.Context, domain.HumanGateDecisionRequest) (domain.HumanGateOwnerApplication, error)
	PrepareSignal(context.Context, domain.SignalPreparation) (domain.SignalPreparation, error)
	BeginSignalAttempt(context.Context, string, time.Time) (domain.SignalPreparation, error)
	FinalizeSignalAttempt(context.Context, domain.SignalIntent, domain.SignalReceipt, int) error
}

type HumanGateOwnerConflictRepository interface {
	RecordHumanGateOwnerConflict(context.Context, domain.HumanGateApplyReceipt) error
}

type WorkflowSignaler interface {
	Signal(context.Context, domain.SignalRequest) (domain.SignalObservation, error)
}

type HumanGateOwnerApplier interface {
	ApplyHumanGateDecision(context.Context, Actor, domain.HumanGateOwnerApplication) (domain.HumanGateOwnerResult, error)
}

type SignalConfig struct {
	Now   func() time.Time
	NewID func() string
	Owner HumanGateOwnerApplier
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
	if service == nil || service.repository == nil || service.signaler == nil || service.config.Now == nil || service.config.NewID == nil || service.config.Owner == nil ||
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
	prepared, found, err := service.repository.FindSignalPreparation(ctx, command.WorkspaceID, command.IdempotencyKey)
	if err != nil {
		return domain.SignalIntent{}, normalizeError(err)
	}
	if !found {
		application, resolveErr := service.repository.ResolveHumanGateOwnerApplication(ctx, domain.HumanGateDecisionRequest{
			WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
			HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
			SubjectRevision: command.SubjectRevision, Decision: command.Decision,
		})
		if resolveErr != nil {
			return domain.SignalIntent{}, normalizeError(resolveErr)
		}
		if !validHumanGateOwnerApplication(command, application) {
			return domain.SignalIntent{}, errors.New("workflow human gate owner application has drifted")
		}
		var owner domain.HumanGateOwnerResult
		if command.Decision == "approved" || command.Decision == "selected" {
			owner, err = service.config.Owner.ApplyHumanGateDecision(ctx, actor, application)
			if err != nil {
				normalizedErr := normalizeError(err)
				var typed *Error
				conflicts, supported := service.repository.(HumanGateOwnerConflictRepository)
				if supported && errors.As(normalizedErr, &typed) && typed.Status == 409 {
					now := service.config.Now().UTC()
					recordErr := conflicts.RecordHumanGateOwnerConflict(ctx, domain.HumanGateApplyReceipt{
						ID:          stableID("workflow-human-gate-apply", command.WorkspaceID, command.ReviewDecisionID),
						WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
						HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
						SubjectRevision: command.SubjectRevision, Decision: command.Decision,
						Status: "conflict", ConflictCode: typed.Code, CreatedBy: actor.UserID, CreatedAt: now,
					})
					if recordErr != nil {
						return domain.SignalIntent{}, normalizeError(recordErr)
					}
				}
				return domain.SignalIntent{}, normalizedErr
			}
			owner, err = normalizeHumanGateOwnerResult(application, owner)
			if err != nil {
				return domain.SignalIntent{}, err
			}
		}
		now := service.config.Now().UTC()
		intentID := stableID("workflow-signal-intent", command.WorkspaceID, command.WorkflowRunID, command.ReviewDecisionID)
		desired := domain.SignalPreparation{
			ApplyReceipt: domain.HumanGateApplyReceipt{
				ID:          stableID("workflow-human-gate-apply", command.WorkspaceID, command.ReviewDecisionID),
				WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
				HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
				SubjectRevision: command.SubjectRevision, Decision: command.Decision,
				OwnerReceiptID: owner.ReceiptID, OwnerOperation: owner.Operation, Output: owner.Output, OutputHash: owner.OutputHash,
				CreatedBy: actor.UserID, CreatedAt: now,
			},
			Intent: domain.SignalIntent{
				ID: intentID, WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
				NodeRunID: command.NodeRunID, HumanTaskID: command.HumanTaskID, ReviewDecisionID: command.ReviewDecisionID,
				IdempotencyKey: command.IdempotencyKey, CommandInputHash: commandInputHash,
				SignalID: stableID("workflow-human-gate-signal", intentID), Decision: command.Decision,
				SubjectRevision: command.SubjectRevision, Status: "pending", Revision: 1,
				CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
			},
		}
		if command.Decision == "approved" || command.Decision == "selected" {
			desired.ApplyReceipt.Status = "completed"
		} else {
			desired.ApplyReceipt.Status = "not_required"
		}
		prepared, err = service.repository.PrepareSignal(ctx, desired)
		if err != nil {
			return domain.SignalIntent{}, normalizeError(err)
		}
	}
	request, err := NewSignalRequest(prepared)
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

func NewSignalRequest(prepared domain.SignalPreparation) (domain.SignalRequest, error) {
	intent, apply := prepared.Intent, prepared.ApplyReceipt
	if apply.WorkspaceID == "" || apply.WorkspaceID != intent.WorkspaceID ||
		apply.WorkflowRunID == "" || apply.WorkflowRunID != intent.WorkflowRunID ||
		apply.NodeRunID == "" || apply.NodeRunID != intent.NodeRunID ||
		apply.HumanTaskID == "" || apply.HumanTaskID != intent.HumanTaskID ||
		apply.ReviewDecisionID == "" || apply.ReviewDecisionID != intent.ReviewDecisionID ||
		apply.SubjectRevision < 1 || apply.SubjectRevision != intent.SubjectRevision || apply.Decision != intent.Decision {
		return domain.SignalRequest{}, errors.New("workflow human gate signal facts have drifted")
	}
	request := domain.SignalRequest{
		TemporalWorkflowID: intent.TemporalWorkflowID, SignalID: intent.SignalID, SignalIntentID: intent.ID,
		WorkflowRunID: intent.WorkflowRunID, NodeRunID: intent.NodeRunID, Decision: strings.ToUpper(intent.Decision),
		OwnerReceiptID: apply.OwnerReceiptID, Output: apply.Output, OutputHash: apply.OutputHash,
	}
	if intent.Decision == "approved" || intent.Decision == "selected" {
		normalized, _, outputHash, outputErr := domain.BuildNodeOutput(apply.Output)
		if outputErr != nil || apply.Status != "completed" || apply.ConflictCode != "" ||
			apply.OwnerReceiptID == "" || apply.OutputHash != outputHash {
			return domain.SignalRequest{}, errors.New("workflow human gate owner evidence is invalid")
		}
		request.Output = normalized
	} else if apply.Status != "not_required" || apply.ConflictCode != "" || apply.OwnerReceiptID != "" ||
		apply.OutputHash != "" || apply.Output.SchemaVersion != "" || len(apply.Output.Bindings) != 0 {
		return domain.SignalRequest{}, errors.New("workflow rejected human gate has owner output")
	}
	inputHash, err := platformcommand.InputHash(request)
	if err != nil {
		return domain.SignalRequest{}, err
	}
	request.InputHash = inputHash
	return request, nil
}

func normalizeHumanGateOwnerResult(
	application domain.HumanGateOwnerApplication,
	result domain.HumanGateOwnerResult,
) (domain.HumanGateOwnerResult, error) {
	normalized, _, outputHash, err := domain.BuildNodeOutput(result.Output)
	if err != nil || strings.TrimSpace(result.ReceiptID) == "" || strings.TrimSpace(result.Operation) == "" ||
		result.OutputHash != outputHash || len(normalized.Bindings) != 1 {
		return domain.HumanGateOwnerResult{}, errors.New("human gate owner returned invalid application evidence")
	}
	binding := normalized.Bindings[0]
	if binding.Port != application.OutputPort || binding.ValueType != application.OutputValueType ||
		!domain.HumanGateOutputMatchesCandidate(application.Executor, application.Candidate, binding) {
		return domain.HumanGateOwnerResult{}, errors.New("human gate owner output does not match the frozen candidate")
	}
	result.ReceiptID, result.Operation, result.Output = strings.TrimSpace(result.ReceiptID), strings.TrimSpace(result.Operation), normalized
	return result, nil
}

func validHumanGateOwnerApplication(
	command SignalHumanGateCommand,
	application domain.HumanGateOwnerApplication,
) bool {
	return application.WorkspaceID == command.WorkspaceID && strings.TrimSpace(application.ProjectID) != "" &&
		application.WorkflowRunID == command.WorkflowRunID && application.NodeRunID == command.NodeRunID &&
		application.HumanTaskID == command.HumanTaskID && application.ReviewDecisionID == command.ReviewDecisionID &&
		application.SubjectRevision == command.SubjectRevision && application.Decision == command.Decision &&
		strings.TrimSpace(application.Executor) != "" && strings.TrimSpace(application.OutputPort) != "" &&
		strings.TrimSpace(application.OutputValueType) != "" &&
		application.Candidate.SourceKind == domain.NodeInputSourceNodeOutput &&
		strings.TrimSpace(application.Candidate.ReferenceID) != "" &&
		strings.TrimSpace(application.Candidate.ReferenceVersion) != "" && len(application.Candidate.ContentHash) == 64
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
