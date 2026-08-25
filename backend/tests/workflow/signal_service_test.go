package workflow_test

import (
	"context"
	"sync"
	"testing"
	"time"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestHumanGateSignalReusesStableIdentityUntilUnknownIsReconciled(t *testing.T) {
	now := time.Date(2026, time.August, 26, 2, 0, 0, 0, time.UTC)
	repository := newSignalRepository()
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
		requests[0].InputHash == "" || len(repository.receipts) != 2 {
		t.Fatalf("signal retry identities = requests %#v receipts %#v", requests, repository.receipts)
	}
}

type signalRepository struct {
	mu       sync.Mutex
	prepared workflow.SignalPreparation
	receipts []workflow.SignalReceipt
}

func newSignalRepository() *signalRepository { return &signalRepository{} }

func (repo *signalRepository) PrepareSignal(
	_ context.Context,
	desired workflow.SignalPreparation,
) (workflow.SignalPreparation, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.prepared.Intent.ID == "" {
		repo.prepared = desired
	}
	return repo.prepared, nil
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
