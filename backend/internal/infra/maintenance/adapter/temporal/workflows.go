// Package temporal exposes infrastructure maintenance as durable Go workflows.
package temporal

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// FlowTaskQueue is the existing Go workflow and database activity queue.
const FlowTaskQueue = "flow"

// OutboxPartitionMaintenanceWorkflow prepares writable months and removes empty history.
func OutboxPartitionMaintenanceWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, partitionActivityOptions())
	return workflow.ExecuteActivity(ctx, "MaintainPartitions").Get(ctx, nil)
}

// OutboxCleanupWorkflow removes published events after their retention period.
func OutboxCleanupWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, cleanupActivityOptions())
	return workflow.ExecuteActivity(ctx, "PruneOutbox").Get(ctx, nil)
}

// ProcessedEventCleanupWorkflow removes expired consumer deduplication markers.
func ProcessedEventCleanupWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, cleanupActivityOptions())
	return workflow.ExecuteActivity(ctx, "PruneProcessedEvents").Get(ctx, nil)
}

// Register adds the maintenance workflows and activities to the shared flow worker.
func Register(w worker.Worker, activities *Activities) {
	w.RegisterWorkflow(OutboxPartitionMaintenanceWorkflow)
	w.RegisterWorkflow(OutboxCleanupWorkflow)
	w.RegisterWorkflow(ProcessedEventCleanupWorkflow)
	w.RegisterActivity(activities)
}

func partitionActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second,
			MaximumInterval: time.Minute,
		},
	}
}

func cleanupActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second,
			MaximumInterval: time.Minute,
		},
	}
}
