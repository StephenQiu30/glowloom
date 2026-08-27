package workflow_test

import (
	"context"
	"errors"
	"testing"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestHumanGateCoordinatorResumesOnlyFromPersistedDecisionIdentity(t *testing.T) {
	decision := workflowdomain.HumanGateReviewDecision{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", NodeRunID: "node-1",
		HumanTaskID: "task-1", ReviewDecisionID: "decision-1", SubjectRevision: 3,
		SubjectHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Decision: "rejected",
	}
	decisions := &humanGateDecisionReader{decision: decision}
	statuses := &humanGateStatusRepository{status: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: "decision-1", DecisionStatus: "recorded",
		OwnerApplyStatus: "pending", WorkflowResumeStatus: "pending",
	}}
	signals := &humanGateSignalService{intent: workflowdomain.SignalIntent{Status: "completed"}, statuses: statuses}
	coordinator := workflowapp.NewHumanGateCoordinator(decisions, signals, statuses)
	result, err := coordinator.ResumeHumanGate(context.Background(), workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}, "decision-1")
	if err != nil || result.WorkflowResumeStatus != "completed" || signals.command.IdempotencyKey != "human-gate-decision:decision-1" ||
		signals.command.WorkspaceID != decision.WorkspaceID || signals.command.HumanTaskID != decision.HumanTaskID ||
		signals.command.Decision != decision.Decision {
		t.Fatalf("resume result=%#v err=%v command=%#v", result, err, signals.command)
	}
	if decisions.decisionID != "decision-1" || statuses.decisionID != "decision-1" {
		t.Fatalf("coordinator identities decisions=%q statuses=%q", decisions.decisionID, statuses.decisionID)
	}
}

func TestHumanGateCoordinatorDoesNotRetryPersistedOwnerConflict(t *testing.T) {
	decisions := &humanGateDecisionReader{decision: workflowdomain.HumanGateReviewDecision{
		WorkspaceID: "workspace-2", WorkflowRunID: "run-2", NodeRunID: "node-2",
		HumanTaskID: "task-2", ReviewDecisionID: "decision-2", SubjectRevision: 1,
		SubjectHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Decision: "approved",
	}}
	signals := &humanGateSignalService{}
	statuses := &humanGateStatusRepository{status: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: "decision-2", DecisionStatus: "recorded",
		OwnerApplyStatus: "conflict", WorkflowResumeStatus: "pending", ConflictCode: "owner_baseline_conflict",
	}}
	coordinator := workflowapp.NewHumanGateCoordinator(decisions, signals, statuses)
	result, err := coordinator.ResumeHumanGate(context.Background(), workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}, "decision-2")
	var typed *workflowapp.Error
	if !errors.As(err, &typed) || typed.Status != 409 || result.OwnerApplyStatus != "conflict" || signals.calls != 0 {
		t.Fatalf("conflict result=%#v err=%v signal calls=%d", result, err, signals.calls)
	}
}

type humanGateDecisionReader struct {
	decision   workflowdomain.HumanGateReviewDecision
	decisionID string
}

func (reader *humanGateDecisionReader) GetHumanGateDecision(_ context.Context, _ workflowapp.Actor, decisionID string) (workflowdomain.HumanGateReviewDecision, error) {
	reader.decisionID = decisionID
	return reader.decision, nil
}

type humanGateSignalService struct {
	intent   workflowdomain.SignalIntent
	command  workflowapp.SignalHumanGateCommand
	calls    int
	statuses *humanGateStatusRepository
}

func (service *humanGateSignalService) SignalHumanGate(_ context.Context, _ workflowapp.Actor, command workflowapp.SignalHumanGateCommand) (workflowdomain.SignalIntent, error) {
	service.command = command
	service.calls++
	if service.statuses != nil {
		service.statuses.status.OwnerApplyStatus = "not_required"
		service.statuses.status.WorkflowResumeStatus = service.intent.Status
	}
	return service.intent, nil
}

type humanGateStatusRepository struct {
	status     workflowdomain.HumanGateCoordination
	decisionID string
}

func (repository *humanGateStatusRepository) GetHumanGateCoordination(_ context.Context, _ string, decisionID string) (workflowdomain.HumanGateCoordination, error) {
	repository.decisionID = decisionID
	return repository.status, nil
}
