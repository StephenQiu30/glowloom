package workflow

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// QuoteExpiryWorkflow expires unconfirmed quotes through one bounded Activity.
func QuoteExpiryWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second, MaximumInterval: time.Minute,
		},
	})
	return workflow.ExecuteActivity(ctx, "flow.ExpireQuotes").Get(ctx, nil)
}

// QuoteExpiryActivities adapts the operation use case to the flow queue.
type QuoteExpiryActivities struct {
	service *application.QuoteExpiryService
}

// NewQuoteExpiryActivities injects the bounded quote-expiry use case.
func NewQuoteExpiryActivities(service *application.QuoteExpiryService) *QuoteExpiryActivities {
	return &QuoteExpiryActivities{service: service}
}

// ExpireQuotes scans at a fixed cutoff and heartbeats after each database batch.
func (a *QuoteExpiryActivities) ExpireQuotes(ctx context.Context) (application.ExpiredQuotes, error) {
	return a.service.Run(ctx, time.Now().UTC(), func(total application.ExpiredQuotes) {
		activity.RecordHeartbeat(ctx, total)
	})
}

// RegisterQuoteExpiry adds this schedule's workflow and Activity to flow.
func RegisterQuoteExpiry(w worker.Worker, activities *QuoteExpiryActivities) {
	w.RegisterWorkflowWithOptions(QuoteExpiryWorkflow, workflow.RegisterOptions{Name: "QuoteExpiryWorkflow"})
	w.RegisterActivityWithOptions(activities.ExpireQuotes, activity.RegisterOptions{Name: "flow.ExpireQuotes"})
}
