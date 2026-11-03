package workflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// SignalClient is the Temporal operation used by the durable control consumer.
type SignalClient interface {
	SignalWorkflow(context.Context, string, string, string, interface{}) error
}

// OriginalDeliveryReader verifies that only staged delivery is needed.
type OriginalDeliveryReader interface {
	OriginalDeliveryRecoveryRequired(context.Context, application.WorkflowControlDelivery) (bool, error)
}

// Control sends fixed signals or starts original delivery without a generation path.
type Control struct {
	client   SignalClient
	recovery OriginalDeliveryReader
}

// NewControl injects the relay's Temporal client.
func NewControl(workflowClient SignalClient, readers ...OriginalDeliveryReader) *Control {
	c := &Control{client: workflowClient}
	if len(readers) > 0 {
		c.recovery = readers[0]
	}
	return c
}

// SignalControl never terminates a workflow or reports provider cancellation success.
func (c *Control) SignalControl(ctx context.Context, delivery application.WorkflowControlDelivery) error {
	kind, id, action := delivery.TargetType, delivery.TargetID, delivery.Action
	validControl := kind == "operation" && (action == "cancel" || action == "recover") || kind == "batch" && (action == "cancel" || action == "resume")
	if c == nil || c.client == nil || id == uuid.Nil || !validControl {
		return ErrInvalidOperationInput
	}
	if action == "recover" && c.recovery != nil {
		terminal, err := c.recovery.OriginalDeliveryRecoveryRequired(ctx, delivery)
		if err != nil {
			return err
		}
		if terminal {
			starter, ok := c.client.(interface {
				ExecuteWorkflow(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
			})
			if !ok {
				return ErrInvalidOperationInput
			}
			_, err := starter.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
				ID: "operation-delivery/" + id.String(), TaskQueue: "flow",
				WorkflowIDReusePolicy:                    enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
				WorkflowIDConflictPolicy:                 enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL,
				WorkflowExecutionErrorWhenAlreadyStarted: true,
			}, "OriginalDeliveryRecoveryWorkflow", delivery)
			// Another request may own the running archive. Redeliver this
			// intent after it finishes or loses authority; the relay skips ready results.
			return err
		}
	}
	var signal any
	if action == "recover" {
		signal = originalCheckSignal{CheckOriginal: true}
	}
	if err := c.client.SignalWorkflow(ctx, kind+"/"+id.String(), "", action, signal); err != nil {
		return fmt.Errorf("signal workflow control: %w", err)
	}
	return nil
}
