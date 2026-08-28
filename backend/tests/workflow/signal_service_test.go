package workflow_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestHumanGateSignalReusesStableIdentityUntilUnknownIsReconciled(t *testing.T) {
	now := time.Date(2026, time.August, 26, 2, 0, 0, 0, time.UTC)
	repository := newSignalRepository()
	ownerOutput, _, ownerOutputHash, ownerOutputErr := workflow.BuildNodeOutput(workflow.NodeOutputSnapshot{
		SchemaVersion: workflow.NodeOutputSchemaVersion,
		Bindings: []workflow.NodeOutputBinding{{
			Port: "bible", ValueType: "production_bible_version", ReferenceID: "00000000-0000-0000-0000-000000000335",
			ReferenceVersion: "1", ContentHash: strings.Repeat("d", 64),
		}},
	})
	if ownerOutputErr != nil {
		t.Fatal(ownerOutputErr)
	}
	owner := &scriptedGateOwner{result: workflow.HumanGateOwnerResult{
		ReceiptID: "00000000-0000-0000-0000-000000000334", Operation: "production_bible.confirm",
		Output: ownerOutput, OutputHash: ownerOutputHash,
	}}
	repository.application = workflow.HumanGateOwnerApplication{
		ProjectID: "project-1", Executor: "gate.production_bible_review", Decision: "approved",
		Candidate: workflow.NodeInputBinding{
			Port: "candidate", ValueType: "story_reconciliation_candidate", SourceKind: workflow.NodeInputSourceNodeOutput,
			ReferenceID: "00000000-0000-0000-0000-000000000333", ReferenceVersion: "1", ContentHash: strings.Repeat("c", 64),
		},
		OutputPort: "bible", OutputValueType: "production_bible_version",
	}
	signaler := &scriptedSignaler{outcomes: []workflow.SignalObservation{
		{Outcome: workflow.SignalOutcomeUnknown},
		{Outcome: workflow.SignalOutcomeAlreadyApplied, ObservedInputHash: "match_request"},
	}}
	id := 0
	service := workflowapp.NewSignalService(repository, signaler, workflowapp.SignalConfig{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string {
			id++
			return "generated-" + string(rune('0'+id))
		},
		Owner: owner,
	})
	command := workflowapp.SignalHumanGateCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", NodeRunID: "node-run-1",
		HumanTaskID: "task-1", ReviewDecisionID: "decision-1", SubjectRevision: 7,
		Decision: "approved", IdempotencyKey: "signal-decision-1",
	}
	actor := workflowapp.Actor{UserID: "reviewer-1", TokenVersion: 1}

	unknown, err := service.SignalHumanGate(context.Background(), actor, command)
	if err != nil || unknown.Status != "unknown" || unknown.AttemptNo != 1 {
		t.Fatalf("record unknown signal: intent=%#v err=%v", unknown, err)
	}
	completed, err := service.SignalHumanGate(context.Background(), actor, command)
	if err != nil || completed.Status != "completed" || completed.AttemptNo != 2 {
		t.Fatalf("reconcile human gate signal: intent=%#v err=%v", completed, err)
	}
	requests := signaler.Requests()
	if len(requests) != 2 || requests[0].SignalIntentID != requests[1].SignalIntentID ||
		requests[0].SignalID != requests[1].SignalID || requests[0].InputHash != requests[1].InputHash ||
		requests[0].InputHash == "" || requests[0].OwnerReceiptID != owner.result.ReceiptID ||
		requests[0].OutputHash != ownerOutputHash || len(repository.receipts) != 2 || owner.calls != 1 {
		t.Fatalf("signal retry identities = requests %#v receipts %#v", requests, repository.receipts)
	}
	drifted := command
	drifted.Decision = "selected"
	if _, err = service.SignalHumanGate(context.Background(), actor, drifted); err == nil {
		t.Fatal("signal replay accepted the same idempotency key with different input")
	}
	if owner.calls != 1 || len(signaler.Requests()) != 2 {
		t.Fatalf("drifted signal replay produced effects: owner calls=%d signal requests=%d", owner.calls, len(signaler.Requests()))
	}
}

func TestRejectedEpisodePlanGateResumesWithoutCallingThePlanningOwner(t *testing.T) {
	now := time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC)
	repository := newSignalRepository()
	repository.application = workflow.HumanGateOwnerApplication{
		ProjectID: "project-1", Executor: "gate.episode_plan_review", Decision: "rejected",
		Candidate: workflow.NodeInputBinding{
			Port: "candidate", ValueType: "episode_segmentation_candidate", SourceKind: workflow.NodeInputSourceNodeOutput,
			ReferenceID: "00000000-0000-0000-0000-000000000401", ReferenceVersion: "1", ContentHash: strings.Repeat("e", 64),
		},
		OutputPort: "episodes", OutputValueType: "episode_set",
	}
	owner := &scriptedGateOwner{}
	signaler := &scriptedSignaler{outcomes: []workflow.SignalObservation{{
		Outcome: workflow.SignalOutcomeSignaled, ObservedInputHash: "match_request",
	}}}
	service := workflowapp.NewSignalService(repository, signaler, workflowapp.SignalConfig{
		Now:   func() time.Time { now = now.Add(time.Second); return now },
		NewID: func() string { return "00000000-0000-0000-0000-000000000402" }, Owner: owner,
	})
	intent, err := service.SignalHumanGate(context.Background(), workflowapp.Actor{
		UserID: "reviewer-1", TokenVersion: 1,
	}, workflowapp.SignalHumanGateCommand{
		WorkspaceID: "workspace-1", WorkflowRunID: "run-1", NodeRunID: "node-run-1",
		HumanTaskID: "task-1", ReviewDecisionID: "decision-1", SubjectRevision: 1,
		Decision: "rejected", IdempotencyKey: "episode-plan-rejected",
	})
	if err != nil || intent.Status != "completed" || owner.calls != 0 ||
		repository.prepared.ApplyReceipt.Status != "not_required" || repository.prepared.ApplyReceipt.OwnerReceiptID != "" ||
		repository.prepared.ApplyReceipt.OutputHash != "" || len(repository.prepared.ApplyReceipt.Output.Bindings) != 0 {
		t.Fatalf("rejected Episode Plan Gate produced an owner effect: intent=%#v apply=%#v owner_calls=%d err=%v",
			intent, repository.prepared.ApplyReceipt, owner.calls, err)
	}
}

func TestNonApprovedStoryboardIntentGateResumesWithoutFreezingOrCallingOwner(t *testing.T) {
	for _, decision := range []string{"rejected", "changes_requested"} {
		t.Run(decision, func(t *testing.T) {
			now := time.Date(2026, time.August, 28, 18, 0, 0, 0, time.UTC)
			repository := newSignalRepository()
			repository.application = workflow.HumanGateOwnerApplication{
				ProjectID: "project-1", Executor: "gate.storyboard_review", Decision: decision,
				Candidate: workflow.NodeInputBinding{
					Port: "candidate", ValueType: "storyboard_intent_candidate_set", SourceKind: workflow.NodeInputSourceNodeOutput,
					ReferenceID: "00000000-0000-0000-0000-000000000411", ReferenceVersion: "1",
					ContentHash: strings.Repeat("f", 64),
				},
				OutputPort: "intents", OutputValueType: "approved_storyboard_intents",
			}
			owner := &scriptedGateOwner{}
			signaler := &scriptedSignaler{outcomes: []workflow.SignalObservation{{
				Outcome: workflow.SignalOutcomeSignaled, ObservedInputHash: "match_request",
			}}}
			service := workflowapp.NewSignalService(repository, signaler, workflowapp.SignalConfig{
				Now:   func() time.Time { now = now.Add(time.Second); return now },
				NewID: func() string { return "00000000-0000-0000-0000-000000000412" }, Owner: owner,
			})
			intent, err := service.SignalHumanGate(context.Background(), workflowapp.Actor{
				UserID: "reviewer-1", TokenVersion: 1,
			}, workflowapp.SignalHumanGateCommand{
				WorkspaceID: "workspace-1", WorkflowRunID: "run-1", NodeRunID: "node-run-1",
				HumanTaskID: "task-1", ReviewDecisionID: "decision-1", SubjectRevision: 1,
				Decision: decision, IdempotencyKey: "storyboard-intent-" + decision,
			})
			if err != nil || intent.Status != "completed" || owner.calls != 0 ||
				repository.prepared.ApplyReceipt.Status != "not_required" ||
				repository.prepared.ApplyReceipt.OwnerReceiptID != "" ||
				repository.prepared.ApplyReceipt.OwnerOperation != "" ||
				repository.prepared.ApplyReceipt.OutputHash != "" || len(repository.prepared.ApplyReceipt.Output.Bindings) != 0 {
				t.Fatalf("non-approved Storyboard Intent Gate produced an owner effect: intent=%#v apply=%#v owner_calls=%d err=%v",
					intent, repository.prepared.ApplyReceipt, owner.calls, err)
			}
		})
	}
}

type signalRepository struct {
	mu          sync.Mutex
	prepared    workflow.SignalPreparation
	receipts    []workflow.SignalReceipt
	application workflow.HumanGateOwnerApplication
}

func newSignalRepository() *signalRepository { return &signalRepository{} }

func (repo *signalRepository) FindSignalPreparation(
	_ context.Context,
	workspaceID string,
	idempotencyKey string,
) (workflow.SignalPreparation, bool, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	found := repo.prepared.Intent.WorkspaceID == workspaceID && repo.prepared.Intent.IdempotencyKey == idempotencyKey
	return repo.prepared, found, nil
}

func (repo *signalRepository) ResolveHumanGateOwnerApplication(
	_ context.Context,
	request workflow.HumanGateDecisionRequest,
) (workflow.HumanGateOwnerApplication, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	resolved := repo.application
	resolved.WorkspaceID, resolved.WorkflowRunID, resolved.NodeRunID = request.WorkspaceID, request.WorkflowRunID, request.NodeRunID
	resolved.HumanTaskID, resolved.ReviewDecisionID = request.HumanTaskID, request.ReviewDecisionID
	resolved.SubjectRevision, resolved.Decision = request.SubjectRevision, request.Decision
	return resolved, nil
}

func (repo *signalRepository) PrepareSignal(
	_ context.Context,
	desired workflow.SignalPreparation,
) (workflow.SignalPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.prepared.Intent.ID == "" {
		desired.Intent.TemporalWorkflowID = "temporal:" + desired.Intent.WorkflowRunID
		request, err := workflowapp.NewSignalRequest(desired)
		if err != nil {
			return workflow.SignalPreparation{}, err
		}
		desired.Intent.InputHash = request.InputHash
		repo.prepared = desired
	}
	return repo.prepared, nil
}

type scriptedGateOwner struct {
	result workflow.HumanGateOwnerResult
	calls  int
}

func (owner *scriptedGateOwner) ApplyHumanGateDecision(
	_ context.Context,
	_ workflowapp.Actor,
	_ workflow.HumanGateOwnerApplication,
) (workflow.HumanGateOwnerResult, error) {
	owner.calls++
	return owner.result, nil
}

func (repo *signalRepository) BeginSignalAttempt(
	_ context.Context,
	intentID string,
	now time.Time,
) (workflow.SignalPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.prepared.Intent.ID != intentID || repo.prepared.Intent.Status == "completed" || repo.prepared.Intent.Status == "conflict" {
		return repo.prepared, nil
	}
	repo.prepared.Intent.Status = "pending"
	repo.prepared.Intent.AttemptNo++
	repo.prepared.Intent.Revision++
	repo.prepared.Intent.UpdatedAt = now
	return repo.prepared, nil
}

func (repo *signalRepository) FinalizeSignalAttempt(
	_ context.Context,
	intent workflow.SignalIntent,
	receipt workflow.SignalReceipt,
	_ int,
) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.prepared.Intent = intent
	repo.receipts = append(repo.receipts, receipt)
	return nil
}

type scriptedSignaler struct {
	mu       sync.Mutex
	outcomes []workflow.SignalObservation
	requests []workflow.SignalRequest
}

func (signaler *scriptedSignaler) Signal(_ context.Context, request workflow.SignalRequest) (workflow.SignalObservation, error) {
	signaler.mu.Lock()
	defer signaler.mu.Unlock()
	signaler.requests = append(signaler.requests, request)
	observation := signaler.outcomes[0]
	signaler.outcomes = signaler.outcomes[1:]
	if observation.ObservedInputHash == "match_request" {
		observation.ObservedInputHash = request.InputHash
	}
	return observation, nil
}

func (signaler *scriptedSignaler) Requests() []workflow.SignalRequest {
	signaler.mu.Lock()
	defer signaler.mu.Unlock()
	return append([]workflow.SignalRequest(nil), signaler.requests...)
}
