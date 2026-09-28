package operation_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestOperationWorkflowNormalizesProviderFailureCode(t *testing.T) {
	for _, test := range []struct {
		name  string
		input *workflow.ProviderError
		want  string
	}{
		{name: "missing", want: "provider:failed"},
		{name: "normalized", input: &workflow.ProviderError{Code: " Rate Limited / 429 "}, want: "provider:rate_limited___429"},
		{name: "stable", input: &workflow.ProviderError{Code: "mock_failure"}, want: "provider:mock_failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			loaded := workflowFixture()
			id := loaded.Operation.ID.String()
			suite := &testsuite.WorkflowTestSuite{}
			env := suite.NewTestWorkflowEnvironment()
			registerOperationMockActivities(env)
			env.OnActivity("flow.LoadOperation", mock.Anything, id).Return(loaded, nil).Once()
			env.OnActivity("flow.CheckConsent", mock.Anything, id).Return(nil).Once()
			env.OnActivity("flow.Transition", mock.Anything, mock.Anything).
				Return(domain.StatusSubmitting, nil).Once()
			env.OnActivity("flow.BeginProviderCall", mock.Anything, mock.Anything).Return(nil).Once()
			env.OnActivity("provider.submit", mock.Anything, mock.Anything).
				Return(workflow.ProviderSubmitOutput{Outcome: workflow.ProviderSubmitRejected, Error: test.input}, nil).Once()
			env.OnActivity("flow.CompleteProviderCall", mock.Anything, mock.Anything).Return(nil).Once()
			env.OnActivity("flow.SettleOperation", mock.Anything, mock.MatchedBy(func(input workflow.SettlementInput) bool {
				return input.OperationID == id && input.To == domain.StatusFailed &&
					input.ActualCostMicros == 0 && input.FailureCode == test.want
			})).Return(nil).Once()

			env.ExecuteWorkflow(workflow.OperationWorkflow, workflow.OperationInput{OperationID: id})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatalf("provider rejection workflow: %v", err)
			}
			env.AssertExpectations(t)
		})
	}
}
