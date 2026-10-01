package operation_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func syncWorkflowFixture() application.WorkflowOperation {
	loaded := workflowFixture()
	loaded.Provider.AdapterKey = "codex"
	loaded.Provider.Queue = "agent.codex"
	loaded.Provider.SupportsQuery = false
	loaded.Provider.ProviderModelID = "fixture-model"
	loaded.Operation.Params = []byte(`{}`)
	return loaded
}

func TestM1SyncCostAndManualKeepsReservationWithoutQueryOrMockReview(t *testing.T) {
	for _, outcome := range []workflow.ProviderSubmitOutcome{workflow.ProviderSubmitCompleted, workflow.ProviderSubmitUnknown} {
		t.Run(string(outcome), func(t *testing.T) {
			loaded := syncWorkflowFixture()
			id := loaded.Operation.ID.String()
			receipt := codexTestReceipt(application.ProviderDispatchIdentity{ProjectID: loaded.Operation.ProjectID, OperationID: loaded.Operation.ID, Action: "submit", Attempt: 1, RequestKey: loaded.ProviderRequestKey, ModelProfileVersionID: *loaded.Operation.ModelProfileVersionID, PriceRuleVersionID: *loaded.Operation.PriceRuleVersionID})
			suite := &testsuite.WorkflowTestSuite{}
			env := suite.NewTestWorkflowEnvironment()
			registerOperationMockActivities(env)
			registerSyncWorkflowActivities(env)
			env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
			env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
			var transitions []domain.Status
			env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
				transitions = append(transitions, input.To)
				return input.To, nil
			}).Maybe()
			env.OnActivity("flow.BeginProviderCall", mock.Anything, mock.MatchedBy(func(input application.BeginProviderCallInput) bool {
				return input.Attempt == 1 && input.DispatchRequired && input.ProviderTaskID == nil
			})).Return(nil).Once()
			result := workflow.ProviderSubmitOutput{Outcome: outcome}
			if outcome == workflow.ProviderSubmitCompleted {
				result.Receipt = &receipt
			}
			env.OnActivity("provider.submit", mock.Anything, mock.MatchedBy(func(input workflow.ProviderSubmitInput) bool {
				return input.Attempt == 1 && input.ProjectID == loaded.Operation.ProjectID.String() && input.ModelProfileVersionID == receipt.Identity.ModelProfileVersionID.String() && input.PriceRuleVersionID == receipt.Identity.PriceRuleVersionID.String()
			})).Return(result, nil).Once()
			env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.MatchedBy(func(input application.CompleteProviderCallInput) bool {
				return input.State == string(outcome) && input.ProviderTaskID == nil && (outcome != workflow.ProviderSubmitCompleted || input.Receipt != nil)
			})).Return(nil).Once()
			if outcome == workflow.ProviderSubmitCompleted {
				env.OnActivity("flow.LoadProviderCost", mock.Anything, id).Return(application.ProviderCost{}, temporal.NewNonRetryableApplicationError("cost unavailable", "provider_cost_unknown", nil)).Once()
			}
			env.OnActivity("flow.RecoverProviderImage", mock.Anything, id).Return(&receipt, nil).Maybe()
			// A forged task ID/cost cannot establish a trustworthy synchronous charge.
			env.RegisterDelayedCallback(func() {
				env.SignalWorkflow("resolve_manual", workflow.ManualResolution{Outcome: "succeeded", ProviderTaskID: "forged-task"})
			}, time.Second)
			env.OnActivity("flow.LoadProviderCost", mock.Anything, id).Return(application.ProviderCost{}, temporal.NewNonRetryableApplicationError("cost unavailable", "provider_cost_unknown", nil)).Maybe()
			env.RegisterDelayedCallback(env.CancelWorkflow, 2*time.Second)
			env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
			if err := env.GetWorkflowError(); err == nil || !temporal.IsCanceledError(err) {
				t.Fatalf("manual workflow should remain open until cancellation: %v", err)
			}
			want := []domain.Status{domain.StatusSubmitting, domain.StatusUnknown, domain.StatusReconciling, domain.StatusManual}
			if !reflect.DeepEqual(transitions, want) {
				t.Fatalf("cost missing changed state/settlement: %v", transitions)
			}
			env.AssertExpectations(t)
		})
	}
}

func TestM1SyncWorkflowReplayDefaultVersionCannotStartNewProvider(t *testing.T) {
	loaded := syncWorkflowFixture()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	registerSyncWorkflowActivities(env)
	id := loaded.Operation.ID.String()
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	env.OnGetVersion("operation-codex-image-v1", temporalworkflow.DefaultVersion, 1).Return(temporalworkflow.DefaultVersion).Once()
	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err == nil || !strings.Contains(err.Error(), workflow.ErrUnsupportedOperation.Error()) {
		t.Fatalf("default version unexpectedly started synchronous provider: %v", err)
	}
	env.AssertExpectations(t)
}

func TestM1SyncCostAndManualQueuedCancellationRequiresDefiniteNoSend(t *testing.T) {
	loaded := syncWorkflowFixture()
	id := loaded.Operation.ID.String()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	registerSyncWorkflowActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	var states []domain.Status
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
		states = append(states, input.To)
		return input.To, nil
	}).Maybe()
	env.OnActivity("flow.BeginProviderCall", mock.Anything, mock.MatchedBy(func(input application.BeginProviderCallInput) bool { return input.DispatchRequired })).Return(nil).Once()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitNotSubmitted, Error: &workflow.ProviderError{Code: "provider_dispatch_cancelled"}}, nil).Once()
	env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.MatchedBy(func(input application.CompleteProviderCallInput) bool {
		return input.State == "not_submitted" && input.Receipt == nil && input.Usage == nil
	})).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.From == domain.StatusCancelling && input.To == domain.StatusCancelled && input.ActualCostMicros == 0
	})).Return(nil).Once()
	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(states, []domain.Status{domain.StatusSubmitting, domain.StatusCancelling}) {
		t.Fatalf("queued cancellation did not honor definite no-send evidence: %v", states)
	}
	env.AssertExpectations(t)
}

func TestM1SyncCostAndManualExecutedReceiptRejectsNotExecutedSignal(t *testing.T) {
	loaded := syncWorkflowFixture()
	loaded.Operation.Status = domain.StatusManual
	id := loaded.Operation.ID.String()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	registerSyncWorkflowActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckManualNotExecuted", mock.Anything, id).Return(temporal.NewNonRetryableApplicationError("completed receipt cannot release charge", "manual_resolution_unverified", nil)).Once()
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("resolve_manual", workflow.ManualResolution{Outcome: "not_executed"}) }, time.Second)
	env.RegisterDelayedCallback(env.CancelWorkflow, 2*time.Second)
	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err == nil || !temporal.IsCanceledError(err) {
		t.Fatalf("unverified release closed the manual operation: %v", err)
	}
	env.AssertExpectations(t)
}

func registerSyncWorkflowActivities(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterActivityWithOptions(func(context.Context, string) (application.ProviderCost, error) {
		return application.ProviderCost{}, nil
	}, activity.RegisterOptions{Name: "flow.LoadProviderCost"})
	env.RegisterActivityWithOptions(func(context.Context, string) (*application.ProviderReceipt, error) { return nil, nil }, activity.RegisterOptions{Name: "flow.RecoverProviderImage"})
}
