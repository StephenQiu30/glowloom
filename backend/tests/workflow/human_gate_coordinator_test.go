package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestHumanGateCoordinatorResumesOnlyFromPersistedDecisionIdentity(t *testing.T) {
	decision := workflowdomain.HumanGateReviewDecision{
		WorkspaceID: "workspace-1", ProjectID: "project-1", WorkflowRunID: "run-1", NodeRunID: "node-1",
		HumanTaskID: "task-1", ReviewDecisionID: "decision-1", SubjectRevision: 3,
		SubjectHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Decision: "rejected",
		DecisionPayloadHash: emptyReviewDecisionPayloadHash,
	}
	decisions := &humanGateDecisionReader{decision: decision}
	statuses := &humanGateStatusRepository{status: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: "decision-1", DecisionStatus: "recorded",
		OwnerApplyStatus: "pending", WorkflowResumeStatus: "pending",
	}}
	signals := &humanGateSignalService{intent: workflowdomain.SignalIntent{Status: "completed"}, statuses: statuses}
	coordinator := workflowapp.NewHumanGateCoordinator(decisions, signals, statuses, &humanGateRepairService{})
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
		WorkspaceID: "workspace-2", ProjectID: "project-2", WorkflowRunID: "run-2", NodeRunID: "node-2",
		HumanTaskID: "task-2", ReviewDecisionID: "decision-2", SubjectRevision: 1,
		SubjectHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Decision: "approved",
		DecisionPayloadHash: emptyReviewDecisionPayloadHash,
	}}
	signals := &humanGateSignalService{}
	statuses := &humanGateStatusRepository{status: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: "decision-2", DecisionStatus: "recorded",
		OwnerApplyStatus: "conflict", WorkflowResumeStatus: "pending", ConflictCode: "owner_baseline_conflict",
	}}
	coordinator := workflowapp.NewHumanGateCoordinator(decisions, signals, statuses, &humanGateRepairService{})
	result, err := coordinator.ResumeHumanGate(context.Background(), workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}, "decision-2")
	var typed *workflowapp.Error
	if !errors.As(err, &typed) || typed.Status != 409 || result.OwnerApplyStatus != "conflict" || signals.calls != 0 {
		t.Fatalf("conflict result=%#v err=%v signal calls=%d", result, err, signals.calls)
	}
}

func TestHumanGateCoordinatorStartsOneBoundedRepairAfterChangesResume(t *testing.T) {
	decision := workflowdomain.HumanGateReviewDecision{
		WorkspaceID: "workspace-3", ProjectID: "project-3", WorkflowRunID: "run-3", NodeRunID: "node-3",
		HumanTaskID: "task-3", ReviewDecisionID: "decision-3", SubjectRevision: 2,
		SubjectHash: strings.Repeat("c", 64), Decision: "changes_requested",
		SubjectType:         "structure_identity_gate_input",
		DecisionPayloadHash: strings.Repeat("d", 64),
		ChangeRequest: &workflowdomain.StructureIdentityChangeRequest{
			ChangeSpec: workflowdomain.StructureIdentityAllowedChange{Operation: "resolve_mention"},
		},
	}
	statuses := &humanGateStatusRepository{status: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: "decision-3", DecisionStatus: "recorded",
		OwnerApplyStatus: "not_required", WorkflowResumeStatus: "completed",
	}}
	repairs := &humanGateRepairService{run: workflowdomain.WorkflowRun{ID: "repair-run-3"}}
	coordinator := workflowapp.NewHumanGateCoordinator(
		&humanGateDecisionReader{decision: decision},
		&humanGateSignalService{intent: workflowdomain.SignalIntent{Status: "completed"}, statuses: statuses},
		statuses,
		repairs,
	)
	result, err := coordinator.ResumeHumanGate(
		context.Background(), workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}, decision.ReviewDecisionID,
	)
	if err != nil || result.RepairWorkflowRunID != "repair-run-3" || repairs.calls != 1 ||
		repairs.decision.DecisionPayloadHash != decision.DecisionPayloadHash {
		t.Fatalf("repair coordination=%#v repair=%#v calls=%d err=%v", result, repairs.decision, repairs.calls, err)
	}
}

func TestHumanGateCoordinatorStartsProductionWorldRepairAfterChangesResume(t *testing.T) {
	decision := workflowdomain.HumanGateReviewDecision{
		WorkspaceID: "workspace-world", ProjectID: "project-world", WorkflowRunID: "run-world", NodeRunID: "node-world",
		HumanTaskID: "task-world", ReviewDecisionID: "decision-world", SubjectRevision: 1,
		SubjectHash: strings.Repeat("e", 64), Decision: "changes_requested",
		SubjectType:         "production_world_gate_input",
		DecisionPayloadHash: strings.Repeat("f", 64),
		ProductionWorldChangeRequest: &workflowdomain.ProductionWorldChangeRequest{
			ChangeSpec: workflowdomain.ProductionWorldRepairChange{
				Operation: workflowdomain.ProductionWorldRepairReviseInteraction,
			},
		},
	}
	statuses := &humanGateStatusRepository{status: workflowdomain.HumanGateCoordination{
		ReviewDecisionID: decision.ReviewDecisionID, DecisionStatus: "recorded",
		OwnerApplyStatus: "not_required", WorkflowResumeStatus: "completed",
	}}
	repairs := &humanGateRepairService{run: workflowdomain.WorkflowRun{ID: "repair-world"}}
	coordinator := workflowapp.NewHumanGateCoordinator(
		&humanGateDecisionReader{decision: decision},
		&humanGateSignalService{intent: workflowdomain.SignalIntent{Status: "completed"}, statuses: statuses},
		statuses,
		repairs,
	)
	result, err := coordinator.ResumeHumanGate(
		context.Background(), workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}, decision.ReviewDecisionID,
	)
	if err != nil || result.RepairWorkflowRunID != "repair-world" || repairs.calls != 1 ||
		repairs.decision.ProductionWorldChangeRequest == nil || repairs.decision.ChangeRequest != nil {
		t.Fatalf("Production World repair coordination=%#v decision=%#v calls=%d err=%v", result, repairs.decision, repairs.calls, err)
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

type humanGateRepairService struct {
	run      workflowdomain.WorkflowRun
	decision workflowdomain.HumanGateReviewDecision
	calls    int
	err      error
}

func (service *humanGateRepairService) RerunFromHumanGate(
	_ context.Context,
	_ workflowapp.Actor,
	decision workflowdomain.HumanGateReviewDecision,
) (workflowdomain.WorkflowRun, error) {
	service.calls++
	service.decision = decision
	return service.run, service.err
}

func (repository *humanGateStatusRepository) GetHumanGateCoordination(_ context.Context, _ string, decisionID string) (workflowdomain.HumanGateCoordination, error) {
	repository.decisionID = decisionID
	return repository.status, nil
}
