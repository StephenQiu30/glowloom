package workflow

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
)

// TestWorkflowClient is the Temporal start API required by credential scheduling.
type TestWorkflowClient interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
}

// TestStarter delivers the fixed credential test workflow to the flow queue.
type TestStarter struct{ client TestWorkflowClient }

// NewTestStarter injects the configured Temporal client.
func NewTestStarter(c TestWorkflowClient) *TestStarter { return &TestStarter{client: c} }

// StartCredentialTest uses a stable ID so delivery retries cannot duplicate probes.
func (s *TestStarter) StartCredentialTest(ctx context.Context, request application.CredentialTestRequest) error {
	if s == nil || s.client == nil {
		return ErrInvalidTestWorkflow
	}
	input := Input{TestID: request.TestID, ProviderID: request.ProviderID, CredentialID: request.CredentialID, ActorID: request.ActorID, OrgID: request.OrgID, RequestID: request.RequestID}
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: "credential-test/" + request.TestID.String(), TaskQueue: "flow", WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE}, CredentialTestWorkflow, input)
	if err != nil {
		var started *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &started) {
			return nil
		}
		return fmt.Errorf("start credential test workflow: %w", err)
	}
	return nil
}
