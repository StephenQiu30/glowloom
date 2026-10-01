package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

var (
	// ErrInvalidWorkflowControl rejects arbitrary workflow IDs and signal names.
	ErrInvalidWorkflowControl = errors.New("invalid workflow control")
	// ErrWorkflowControlConflict means the task is no longer controllable.
	ErrWorkflowControlConflict = errors.New("workflow control state conflict")
	// ErrWorkflowControlKeyReused means the request key identifies another control.
	ErrWorkflowControlKeyReused = errors.New("workflow control key reused")
	// ErrProviderDispatchCancelled proves a persisted pre-dispatch user cancellation.
	ErrProviderDispatchCancelled = errors.New("provider dispatch cancelled before send")
)

// WorkflowControlInput addresses only a persisted task or batch in its project.
type WorkflowControlInput struct {
	ProjectID  uuid.UUID
	TargetID   uuid.UUID
	TargetType string
	Action     string
	RequestID  string
}

// Validate limits accepted actions before persistence.
func (i WorkflowControlInput) Validate() error {
	key, err := uuid.Parse(i.RequestID)
	if err != nil || key == uuid.Nil || key.String() != i.RequestID || i.ProjectID == uuid.Nil || i.TargetID == uuid.Nil {
		return ErrInvalidWorkflowControl
	}
	if i.TargetType == "operation" && i.Action == "cancel" {
		return nil
	}
	if i.TargetType == "batch" && (i.Action == "cancel" || i.Action == "resume") {
		return nil
	}
	return ErrInvalidWorkflowControl
}

// WorkflowControlResult reports a durable request, not completed cancellation.
type WorkflowControlResult struct {
	RequestID  uuid.UUID `json:"request_id"`
	EventID    uuid.UUID `json:"event_id"`
	TargetID   uuid.UUID `json:"target_id"`
	TargetType string    `json:"target_type"`
	Action     string    `json:"action"`
	Accepted   bool      `json:"accepted"`
}

// WorkflowControlDelivery binds a durable request to its authorized project and actor.
type WorkflowControlDelivery struct {
	EventID, ActorID, OrgID, ProjectID, RequestID, TargetID uuid.UUID
	TargetType, Action                                      string
}

// WorkflowControlStore commits current-rights control, receipt, and Outbox together.
type WorkflowControlStore interface {
	RequestWorkflowControl(context.Context, identityapp.Principal, WorkflowControlInput) (WorkflowControlResult, error)
}

// WorkflowControlCommand validates requests before the transaction authorizes them.
type WorkflowControlCommand struct{ store WorkflowControlStore }

// NewWorkflowControlCommand injects the existing task store.
func NewWorkflowControlCommand(store WorkflowControlStore) *WorkflowControlCommand {
	return &WorkflowControlCommand{store: store}
}

// Execute records a retry-safe intent; only the workflow changes lifecycle and cost.
func (c *WorkflowControlCommand) Execute(ctx context.Context, actor identityapp.Principal, input WorkflowControlInput) (WorkflowControlResult, error) {
	if err := publicActor(actor); err != nil {
		return WorkflowControlResult{}, err
	}
	if c == nil || c.store == nil || input.Validate() != nil {
		return WorkflowControlResult{}, ErrInvalidWorkflowControl
	}
	result, err := c.store.RequestWorkflowControl(ctx, actor, input)
	if err != nil {
		return WorkflowControlResult{}, fmt.Errorf("request workflow control: %w", err)
	}
	return result, nil
}
