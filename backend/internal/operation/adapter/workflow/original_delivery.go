package workflow

import (
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// OriginalDeliveryRecoveryWorkflow only archives original private staging.
// It has no provider activity, quote, confirmation or settlement path.
func OriginalDeliveryRecoveryWorkflow(ctx workflow.Context, input application.WorkflowControlDelivery) error {
	id := input.TargetID
	if id == uuid.Nil || input.ActorID == uuid.Nil || input.RequestID == uuid.Nil || input.EventID == uuid.Nil || input.TargetType != "operation" || input.Action != "recover" {
		return ErrInvalidOperationInput
	}
	if workflow.GetInfo(ctx).WorkflowExecution.ID != "operation-delivery/"+id.String() {
		return ErrInvalidOperationInput
	}
	media := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: "media", StartToCloseTimeout: 20 * time.Minute, RetryPolicy: &temporal.RetryPolicy{InitialInterval: 10 * time.Second, MaximumInterval: 10 * time.Minute}})
	return workflow.ExecuteActivity(media, "media.IngestOriginal", input).Get(media, nil)
}
