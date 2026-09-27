package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func workflowFixture() application.WorkflowOperation {
	operationID, projectID := uuid.New(), uuid.New()
	modelID, priceID := uuid.New(), uuid.New()
	quote, region := int64(100), "domestic"
	created := time.Now().UTC().Add(-time.Minute)
	expires := created.Add(15 * time.Minute)
	prompt := "A lantern in the rain"
	return application.WorkflowOperation{
		Operation: domain.Operation{
			ID: operationID, ProjectID: projectID, TargetType: "free",
			Capability: "image.generate", Mode: "text_to_image",
			ModelProfileVersionID: &modelID, PriceRuleVersionID: &priceID,
			Params:      json.RawMessage(`{"mock_moderation_status":"passed"}`),
			OutputCount: 1, InputHash: "frozen-hash", Origin: "canvas",
			Status: domain.StatusConfirmed, QuoteMicros: &quote,
			QuoteExpiresAt: &expires, Region: &region, CreateTime: created,
		},
		Inputs: []domain.OperationInput{{
			ID: uuid.New(), OperationID: operationID, SeqNo: 0,
			Role: "prompt", RefType: "text", TextValue: &prompt,
		}},
		Provider: application.WorkflowProvider{
			ProviderKey: "mock", AdapterKey: "mock", ProviderModelID: "mock-image",
			Queue: "agent.mock", SupportsQuery: true, ExpectedMaxMS: 60_000,
			Moderation: "platform",
		},
		ProviderRequestKey: "operation/" + operationID.String(),
	}
}

func TestOperationWorkflowUnknownReconcilesWithoutResubmitting(t *testing.T) {
	loaded := workflowFixture()
	workflowID := loaded.Operation.ID
	taskID := "mock-task-1"
	resultURL := "https://provider.example.test/image.png"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, workflowID.String()).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, workflowID.String()).Return(nil).Once()
	var transitions []domain.Status
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(
		func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
			transitions = append(transitions, input.To)
			return input.To, nil
		},
	).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitUnknown}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderRequestKey != nil && *input.ProviderRequestKey == loaded.ProviderRequestKey
	})).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQueryPending, ProviderTaskID: &taskID,
	}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderTaskID != nil && *input.ProviderTaskID == taskID
	})).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID,
		ResultURLs: []string{resultURL},
	}, nil).Once()
	env.OnActivity("media.Ingest", mock.Anything, mock.MatchedBy(func(input map[string]any) bool {
		return input["seq_no"] == float64(1) && input["url"] == resultURL
	})).Return(map[string]any{
		"output_id": uuid.NewString(), "media_asset_id": uuid.NewString(), "kind": "image",
	}, nil).Once()
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return(map[string]any{"status": "passed", "labels": []string{}, "provider": "mock"}, nil).Once()
	env.OnActivity("media.RecordModeration", mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.Anything).Return(nil).Once()

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: workflowID.String()})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("operation workflow: %v", err)
	}
	if !env.IsWorkflowCompleted() {
		t.Fatal("operation workflow did not complete")
	}
	env.AssertExpectations(t)
	want := []domain.Status{
		domain.StatusSubmitting, domain.StatusUnknown, domain.StatusReconciling,
		domain.StatusSubmitted, domain.StatusSucceeded, domain.StatusIngesting,
	}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for index := range want {
		if transitions[index] != want[index] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}

func TestOperationWorkflowRecordsProviderAttemptBeforeDispatchAndSettlement(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).
		Return(domain.StatusSubmitting, nil).Once()
	var order []string
	env.OnActivity("flow.BeginProviderCall", mock.Anything, mock.MatchedBy(func(input application.BeginProviderCallInput) bool {
		return input.OperationID == loaded.Operation.ID && input.Action == "submit" && input.Attempt == 1
	})).Return(func(context.Context, application.BeginProviderCallInput) error {
		order = append(order, "begin")
		return nil
	}).Once()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(func(context.Context, workflow.ProviderSubmitInput) (workflow.ProviderSubmitOutput, error) {
			order = append(order, "submit")
			return workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitRejected}, nil
		}).Once()
	env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.MatchedBy(func(input application.CompleteProviderCallInput) bool {
		return input.OperationID == loaded.Operation.ID && input.Action == "submit" &&
			input.Attempt == 1 && input.Outcome == "error" && input.State == "rejected"
	})).Return(func(context.Context, application.CompleteProviderCallInput) error {
		order = append(order, "complete")
		return nil
	}).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.OperationID == id && input.To == domain.StatusFailed && input.ActualCostMicros == 0
	})).Return(func(context.Context, workflow.SettlementInput) error {
		order = append(order, "settle")
		return nil
	}).Once()

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("rejected provider workflow: %v", err)
	}
	env.AssertExpectations(t)
	if got := strings.Join(order, ","); got != "begin,submit,complete,settle" {
		t.Fatalf("provider attempt order = %q", got)
	}
}

func TestOperationWorkflowUnknownWaitsForManualNotExecutedResolution(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	var transitions []domain.Status
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(
		func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
			transitions = append(transitions, input.To)
			return input.To, nil
		},
	).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitUnknown}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderRequestKey != nil && *input.ProviderRequestKey == loaded.ProviderRequestKey
	})).Return(workflow.ProviderQueryOutput{State: workflow.ProviderQueryNotFound}, nil).Times(6)
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.OperationID == id && input.From == domain.StatusManual &&
			input.To == domain.StatusFailed && input.ActualCostMicros == 0 && input.Retryable
	})).Return(nil).Once()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("resolve_manual", workflow.ManualResolution{Outcome: "not_executed"})
	}, 117*time.Minute)

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("manual not-executed workflow: %v", err)
	}
	env.AssertExpectations(t)
	if len(transitions) != 4 || transitions[0] != domain.StatusSubmitting ||
		transitions[1] != domain.StatusUnknown || transitions[2] != domain.StatusReconciling ||
		transitions[3] != domain.StatusManual {
		t.Fatalf("transitions = %v, want submitting, unknown, reconciling, manual", transitions)
	}
}

func TestOperationWorkflowPollNotFoundReconcilesWithoutResubmitting(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	taskID := "mock-task-poll"
	url := "https://provider.example.test/recovered.png"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	var transitions []domain.Status
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(
		func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
			transitions = append(transitions, input.To)
			return input.To, nil
		},
	).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitAccepted, ProviderTaskID: &taskID}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderTaskID != nil && *input.ProviderTaskID == taskID
	})).Return(workflow.ProviderQueryOutput{State: workflow.ProviderQueryNotFound}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderRequestKey != nil && *input.ProviderRequestKey == loaded.ProviderRequestKey
	})).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
	}, nil).Once()
	env.OnActivity("media.Ingest", mock.Anything, mock.Anything).Return(map[string]any{
		"output_id": uuid.NewString(), "media_asset_id": uuid.NewString(), "kind": "image",
	}, nil).Once()
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return(map[string]any{"status": "passed", "labels": []string{}, "provider": "mock"}, nil).Once()
	env.OnActivity("media.RecordModeration", mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.Anything).Return(nil).Once()

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("poll reconciliation workflow: %v", err)
	}
	env.AssertExpectations(t)
	want := []domain.Status{domain.StatusSubmitting, domain.StatusSubmitted,
		domain.StatusReconciling, domain.StatusSubmitted, domain.StatusSucceeded, domain.StatusIngesting}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for index := range want {
		if transitions[index] != want[index] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}

func TestOperationWorkflowPreSubmitBusinessFailureSettlesWithoutProviderCall(t *testing.T) {
	for _, test := range []struct {
		name     string
		activity string
		code     string
	}{
		{name: "input unavailable", activity: "flow.CheckConsent", code: "input_not_ready"},
		{name: "consent unavailable during transition", activity: "flow.Transition", code: "consent_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			loaded := workflowFixture()
			id := loaded.Operation.ID.String()
			suite := &testsuite.WorkflowTestSuite{}
			env := suite.NewTestWorkflowEnvironment()
			registerOperationMockActivities(env)
			env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
			businessError := temporal.NewNonRetryableApplicationError("pre-submit inputs invalid", test.code, nil)
			if test.activity == "flow.CheckConsent" {
				env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(businessError).Once()
			} else {
				env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
				env.OnActivity("flow.Transition", mock.Anything, mock.Anything).
					Return(domain.Status(""), businessError).Once()
			}
			env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
				return input.OperationID == id && input.From == domain.StatusConfirmed &&
					input.To == domain.StatusFailed && input.ActualCostMicros == 0 &&
					input.FailureCode == test.code
			})).Return(nil).Once()

			env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatalf("pre-submit failure workflow: %v", err)
			}
			env.AssertExpectations(t)
		})
	}
}

func TestOperationWorkflowManualSucceededTakesOverKnownTask(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	taskID := "mock-task-manual"
	url := "https://provider.example.test/manual.png"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	var transitions []domain.Status
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(
		func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
			if input.To == domain.StatusIngesting &&
				(input.ProviderTaskID == nil || *input.ProviderTaskID != taskID) {
				return "", workflow.ErrInvalidProviderResult
			}
			transitions = append(transitions, input.To)
			return input.To, nil
		},
	).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitUnknown}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderRequestKey != nil && *input.ProviderRequestKey == loaded.ProviderRequestKey
	})).Return(workflow.ProviderQueryOutput{State: workflow.ProviderQueryNotFound}, nil).Times(6)
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderRequestKey != nil && *input.ProviderRequestKey == loaded.ProviderRequestKey
	})).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
	}, nil).Times(2)
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderTaskID != nil && *input.ProviderTaskID == taskID
	})).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
	}, nil).Once()
	env.OnActivity("media.Ingest", mock.Anything, mock.Anything).Return(map[string]any{
		"output_id": uuid.NewString(), "media_asset_id": uuid.NewString(), "kind": "image",
	}, nil).Once()
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return(map[string]any{"status": "passed", "labels": []string{}, "provider": "mock"}, nil).Once()
	env.OnActivity("media.RecordModeration", mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.Anything).Return(nil).Once()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("resolve_manual", workflow.ManualResolution{
			Outcome: "succeeded", ProviderTaskID: "another-operation-task",
		})
	}, 117*time.Minute)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("resolve_manual", workflow.ManualResolution{
			Outcome: "succeeded", ProviderTaskID: taskID,
		})
	}, 118*time.Minute)

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("manual success workflow: %v", err)
	}
	env.AssertExpectations(t)
	want := []domain.Status{domain.StatusSubmitting, domain.StatusUnknown, domain.StatusReconciling,
		domain.StatusManual, domain.StatusIngesting}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for index := range want {
		if transitions[index] != want[index] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}

func TestOperationWorkflowPollDeadlineEntersReconciliation(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	taskID := "mock-task-slow"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	var transitions []domain.Status
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(
		func(_ context.Context, input application.TransitionInput) (domain.Status, error) {
			transitions = append(transitions, input.To)
			return input.To, nil
		},
	).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitAccepted, ProviderTaskID: &taskID}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderTaskID != nil && *input.ProviderTaskID == taskID
	})).Return(workflow.ProviderQueryOutput{State: workflow.ProviderQueryRunning}, nil).Times(7)
	env.OnActivity("provider.query", mock.Anything, mock.MatchedBy(func(input workflow.ProviderQueryInput) bool {
		return input.ProviderRequestKey != nil && *input.ProviderRequestKey == loaded.ProviderRequestKey
	})).Return(workflow.ProviderQueryOutput{State: workflow.ProviderQueryNotFound}, nil).Times(6)
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.OperationID == id && input.From == domain.StatusManual &&
			input.To == domain.StatusFailed && input.ActualCostMicros == 0
	})).Return(nil).Once()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("resolve_manual", workflow.ManualResolution{Outcome: "not_executed"})
	}, 119*time.Minute)

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("poll timeout workflow: %v", err)
	}
	env.AssertExpectations(t)
	want := []domain.Status{domain.StatusSubmitting, domain.StatusSubmitted,
		domain.StatusReconciling, domain.StatusManual}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for index := range want {
		if transitions[index] != want[index] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}

func TestOperationWorkflowRetriesModerationWithoutRepeatingProviderOrIngest(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	taskID := "mock-task-review"
	url := "https://provider.example.test/review.png"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(domain.StatusSubmitting, nil).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitAccepted, ProviderTaskID: &taskID}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.Anything).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
	}, nil).Once()
	outputID := uuid.NewString()
	env.OnActivity("media.Ingest", mock.Anything, mock.Anything).Return(map[string]any{
		"output_id": outputID, "media_asset_id": uuid.NewString(), "kind": "image",
	}, nil).Once()
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return((map[string]any)(nil), errors.New("moderation unavailable")).Times(3)
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return(map[string]any{"status": "passed", "labels": []string{}, "provider": "mock"}, nil).Once()
	env.OnActivity("media.RecordModeration", mock.Anything, mock.MatchedBy(func(input map[string]any) bool {
		return input["output_id"] == outputID && input["status"] == "passed"
	})).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.From == domain.StatusIngesting && input.To == domain.StatusCompleted
	})).Return(nil).Once()

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("moderation retry workflow: %v", err)
	}
	env.AssertExpectations(t)
}

func TestOperationWorkflowModerationUnavailableRejectsOutputAndSettles(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	taskID := "mock-task-review-timeout"
	url := "https://provider.example.test/review-timeout.png"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(domain.StatusSubmitting, nil).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitAccepted, ProviderTaskID: &taskID}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.Anything).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
	}, nil).Once()
	outputID := uuid.NewString()
	env.OnActivity("media.Ingest", mock.Anything, mock.Anything).Return(map[string]any{
		"output_id": outputID, "media_asset_id": uuid.NewString(), "kind": "image",
	}, nil).Once()
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return((map[string]any)(nil), errors.New("moderation unavailable")).Maybe()
	env.OnActivity("media.RecordModeration", mock.Anything, mock.MatchedBy(func(input map[string]any) bool {
		return input["output_id"] == outputID && input["status"] == "rejected" &&
			input["reason"] == "moderation_unavailable"
	})).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.From == domain.StatusIngesting && input.To == domain.StatusFailed &&
			input.FailureCode == "moderation_unavailable" && input.Retryable
	})).Return(nil).Once()

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("moderation timeout workflow: %v", err)
	}
	env.AssertExpectations(t)
}

func TestOperationWorkflowRetriesOnlyMediaIngestAfterActivityExhaustion(t *testing.T) {
	loaded := workflowFixture()
	id := loaded.Operation.ID.String()
	taskID := "mock-task-ingest"
	url := "https://provider.example.test/ingest.png"
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	registerOperationMockActivities(env)
	env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
	env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
	env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(domain.StatusSubmitting, nil).Maybe()
	env.OnActivity("provider.submit", mock.Anything, mock.Anything).
		Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitAccepted, ProviderTaskID: &taskID}, nil).Once()
	env.OnActivity("provider.query", mock.Anything, mock.Anything).Return(workflow.ProviderQueryOutput{
		State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
	}, nil).Once()
	env.OnActivity("media.Ingest", mock.Anything, mock.Anything).
		Return((map[string]any)(nil), errors.New("download unavailable")).Times(8)
	env.OnActivity("media.Ingest", mock.Anything, mock.Anything).Return(map[string]any{
		"output_id": uuid.NewString(), "media_asset_id": uuid.NewString(), "kind": "image",
	}, nil).Once()
	env.OnActivity("moderation.check", mock.Anything, mock.Anything).
		Return(map[string]any{"status": "passed", "labels": []string{}, "provider": "mock"}, nil).Once()
	env.OnActivity("media.RecordModeration", mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
		return input.From == domain.StatusIngesting && input.To == domain.StatusCompleted
	})).Return(nil).Once()

	env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("media ingest retry workflow: %v", err)
	}
	env.AssertExpectations(t)
}

func TestOperationWorkflowPermanentMediaFailureSettlesFailed(t *testing.T) {
	for _, failureCode := range []string{"unsupported_media", "provider_result_invalid", "result_expired"} {
		t.Run(failureCode, func(t *testing.T) {
			loaded := workflowFixture()
			id := loaded.Operation.ID.String()
			taskID := "mock-task-permanent-media-failure"
			url := "https://provider.example.test/result"
			suite := &testsuite.WorkflowTestSuite{}
			env := suite.NewTestWorkflowEnvironment()
			registerOperationMockActivities(env)
			env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
			env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
			env.OnActivity("flow.Transition", mock.Anything, mock.Anything).Return(domain.StatusSubmitting, nil).Maybe()
			env.OnActivity("provider.submit", mock.Anything, mock.Anything).
				Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitAccepted, ProviderTaskID: &taskID}, nil).Once()
			env.OnActivity("provider.query", mock.Anything, mock.Anything).Return(workflow.ProviderQueryOutput{
				State: workflow.ProviderQuerySucceeded, ProviderTaskID: &taskID, ResultURLs: []string{url},
			}, nil).Once()
			env.OnActivity("media.Ingest", mock.Anything, mock.Anything).
				Return((map[string]any)(nil), temporal.NewNonRetryableApplicationError("permanent media failure", failureCode, nil)).Once()
			env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
				return input.From == domain.StatusIngesting && input.To == domain.StatusFailed &&
					input.FailureCode == failureCode && input.ActualCostMicros == 0
			})).Return(nil).Once()

			env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatalf("permanent media failure workflow: %v", err)
			}
			env.AssertExpectations(t)
		})
	}
}

func registerOperationMockActivities(env *testsuite.TestWorkflowEnvironment) {
	register := func(name string, fn any) {
		env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name})
	}
	register("flow.LoadOperation", func(context.Context, string) (application.WorkflowOperation, error) {
		return application.WorkflowOperation{}, nil
	})
	register("flow.CheckConsent", func(context.Context, string) error { return nil })
	register("flow.CheckManualNotExecuted", func(context.Context, string) error { return nil })
	register("flow.Transition", func(context.Context, application.TransitionInput) (domain.Status, error) {
		return "", nil
	})
	register("flow.SettleOperation", func(context.Context, workflow.SettlementInput) error { return nil })
	register("flow.BeginProviderCall", func(context.Context, application.BeginProviderCallInput) error { return nil })
	register("flow.CompleteProviderCall", func(context.Context, application.CompleteProviderCallInput) error { return nil })
	register("provider.submit", func(context.Context, workflow.ProviderSubmitInput) (workflow.ProviderSubmitOutput, error) {
		return workflow.ProviderSubmitOutput{}, nil
	})
	register("provider.query", func(context.Context, workflow.ProviderQueryInput) (workflow.ProviderQueryOutput, error) {
		return workflow.ProviderQueryOutput{}, nil
	})
	register("media.Ingest", func(context.Context, map[string]any) (map[string]any, error) {
		return nil, nil
	})
	register("moderation.check", func(context.Context, map[string]any) (map[string]any, error) {
		return nil, nil
	})
	register("media.RecordModeration", func(context.Context, map[string]any) error { return nil })
}
