package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

// Executor is the Temporal call needed by the confirmation starter.
type Executor interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
}

// Starter retries a single operation using a deterministic Temporal workflow ID.
type Starter struct{ client Executor }

// BatchInput addresses a parent workflow by its persisted batch identity.
type BatchInput struct {
	BatchID                 string   `json:"batch_id"`
	AcknowledgedTerminalIDs []string `json:"acknowledged_terminal_ids,omitempty"`
	CancelSignalledIDs      []string `json:"cancel_signalled_ids,omitempty"`
}

// NewStarter injects a Temporal workflow executor.
func NewStarter(workflowClient Executor) *Starter { return &Starter{client: workflowClient} }

// StartOperation treats a previously started run as success, even if it closed.
func (s *Starter) StartOperation(ctx context.Context, operationID uuid.UUID) error {
	if s == nil || s.client == nil || operationID == uuid.Nil {
		return ErrInvalidOperationInput
	}
	workflowID := "operation/" + operationID.String()
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: workflowID, TaskQueue: "flow",
		WorkflowIDReusePolicy:                    enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowIDConflictPolicy:                 enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, OperationWorkflow, OperationInput{OperationID: operationID.String()})
	var started *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &started) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("start operation workflow %s: %w", workflowID, err)
	}
	return nil
}

// StartBatch retries the parent with a deterministic ID; only that parent
// may start its confirmed children under the batch concurrency limit.
func (s *Starter) StartBatch(ctx context.Context, batchID uuid.UUID) error {
	if s == nil || s.client == nil || batchID == uuid.Nil {
		return ErrInvalidOperationInput
	}
	workflowID := "batch/" + batchID.String()
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: workflowID, TaskQueue: "flow",
		WorkflowIDReusePolicy:                    enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowIDConflictPolicy:                 enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, "BatchWorkflow", BatchInput{BatchID: batchID.String()})
	var started *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &started) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("start batch workflow %s: %w", workflowID, err)
	}
	return nil
}
