package workflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SignalClient is the Temporal operation used by the durable control consumer.
type SignalClient interface {
	SignalWorkflow(context.Context, string, string, string, interface{}) error
}

// Control sends fixed signals to workflow IDs derived from persisted identities.
type Control struct{ client SignalClient }

// NewControl injects the relay's Temporal client.
func NewControl(workflowClient SignalClient) *Control { return &Control{client: workflowClient} }

// SignalControl never terminates a workflow or reports provider cancellation success.
func (c *Control) SignalControl(ctx context.Context, kind string, id uuid.UUID, action string) error {
	validControl := kind == "operation" && action == "cancel" || kind == "batch" && (action == "cancel" || action == "resume")
	if c == nil || c.client == nil || id == uuid.Nil || !validControl {
		return ErrInvalidOperationInput
	}
	if err := c.client.SignalWorkflow(ctx, kind+"/"+id.String(), "", action, nil); err != nil {
		return fmt.Errorf("signal workflow control: %w", err)
	}
	return nil
}
