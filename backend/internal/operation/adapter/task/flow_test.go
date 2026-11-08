package task

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type pendingCreditFreezeStore struct{}

func (pendingCreditFreezeStore) LoadWorkflowOperation(context.Context, uuid.UUID) (app.WorkflowOperation, error) {
	return app.WorkflowOperation{}, app.ErrCreditFreezePending
}

func (pendingCreditFreezeStore) TaskEvidence(context.Context, uuid.UUID) (app.TaskEvidence, error) {
	return app.TaskEvidence{}, nil
}

func (pendingCreditFreezeStore) CheckSubmissionInputs(context.Context, uuid.UUID) error { return nil }

func (pendingCreditFreezeStore) TransitionWorkflowOperation(context.Context, app.TransitionInput) (domain.Status, error) {
	return "", nil
}

func (pendingCreditFreezeStore) ScheduleOperationStep(context.Context, string, uuid.UUID) error {
	return nil
}

func (pendingCreditFreezeStore) BeginProviderCall(context.Context, app.BeginProviderCallInput) error {
	return nil
}

func (pendingCreditFreezeStore) CompleteProviderCall(context.Context, app.CompleteProviderCallInput) error {
	return nil
}

func (pendingCreditFreezeStore) CheckManualNotExecuted(context.Context, uuid.UUID) error { return nil }

func (pendingCreditFreezeStore) LoadProviderCost(context.Context, uuid.UUID) (app.ProviderCost, error) {
	return app.ProviderCost{}, nil
}

func (pendingCreditFreezeStore) LoadProviderImageIdentity(context.Context, uuid.UUID) (app.ProviderDispatchIdentity, bool, error) {
	return app.ProviderDispatchIdentity{}, false, nil
}

func TestOperationHandlerWaitsWithoutCountingPendingCreditFreezeAsFailure(t *testing.T) {
	id := uuid.New()
	started := time.Now()
	handler := operationHandler(pendingCreditFreezeStore{}, func(context.Context, app.WorkflowOperation, app.TaskEvidence) (time.Time, error) {
		t.Fatal("operation was dispatched before credit freeze confirmation")
		return time.Time{}, nil
	}, nil)

	next, err := handler(context.Background(), taskruntime.Job{BusinessID: id.String()})
	if err != nil {
		t.Fatalf("pending freeze handler error = %v", err)
	}
	delay := time.Until(next)
	if delay < time.Second || delay > 3*time.Second {
		t.Fatalf("pending freeze reschedule delay = %s", delay)
	}
	if time.Since(started) > time.Second {
		t.Fatal("pending freeze handler blocked instead of rescheduling")
	}
}
