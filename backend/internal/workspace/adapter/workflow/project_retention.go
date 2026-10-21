package workflow

import (
	"context"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// ProjectRetentionActivities adapts bounded cleanup to the existing flow worker.
type ProjectRetentionActivities struct{ service *application.ProjectRetention }

// NewProjectRetentionActivities injects workspace's internal cleanup use case.
func NewProjectRetentionActivities(service *application.ProjectRetention) *ProjectRetentionActivities {
	return &ProjectRetentionActivities{service: service}
}

// RetainExpiredProjects cleans one bounded batch; failed transactions remain retryable.
func (a *ProjectRetentionActivities) RetainExpiredProjects(ctx context.Context) (int, error) {
	return a.service.Run(ctx, 100)
}

// ProjectRetentionWorkflow performs scheduled, durable expired-content cleanup.
func ProjectRetentionWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Minute, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumInterval: time.Minute, MaximumAttempts: 5}})
	for range 10 {
		var processed int
		if err := workflow.ExecuteActivity(ctx, "RetainExpiredProjects").Get(ctx, &processed); err != nil {
			return err
		}
		if processed < 100 {
			return nil
		}
	}
	return workflow.NewContinueAsNewError(ctx, ProjectRetentionWorkflow)
}

// RegisterProjectRetention adds cleanup without installing or enabling a schedule.
func RegisterProjectRetention(w worker.Worker, activities *ProjectRetentionActivities) {
	w.RegisterWorkflow(ProjectRetentionWorkflow)
	w.RegisterActivity(activities)
}
