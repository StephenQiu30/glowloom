package temporal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

const (
	// OutboxCleanupScheduleID is the daily Outbox retention schedule.
	OutboxCleanupScheduleID = "outbox-cleanup"
	// ProcessedEventCleanupScheduleID is the daily consumer marker schedule.
	ProcessedEventCleanupScheduleID = "processed-event-cleanup"
)

// ErrScheduleDrift means an existing schedule differs from the declared action.
var ErrScheduleDrift = errors.New("maintenance schedule differs from declared configuration")

// ScheduleInstaller creates the implemented cleanup schedules in one namespace.
// A prefix isolates local verification from the deployment schedule IDs.
type ScheduleInstaller struct {
	client client.ScheduleClient
	prefix string
}

// NewScheduleInstaller injects the Temporal schedule client and optional ID prefix.
func NewScheduleInstaller(scheduleClient client.ScheduleClient, prefix string) *ScheduleInstaller {
	return &ScheduleInstaller{client: scheduleClient, prefix: prefix}
}

// InstallCleanupSchedules creates the two daily schedules without changing an
// existing schedule's pause state. It rejects mismatched existing definitions.
func (i *ScheduleInstaller) InstallCleanupSchedules(ctx context.Context) error {
	for _, definition := range []struct {
		id       string
		workflow string
		offset   time.Duration
	}{
		{OutboxCleanupScheduleID, "OutboxCleanupWorkflow", 4 * time.Hour},
		{ProcessedEventCleanupScheduleID, "ProcessedEventCleanupWorkflow", 4*time.Hour + 30*time.Minute},
	} {
		options := cleanupScheduleOptions(i.prefix+definition.id, definition.workflow, definition.offset)
		if err := i.ensureSchedule(ctx, options); err != nil {
			return err
		}
	}
	return nil
}

func cleanupScheduleOptions(id, workflowName string, offset time.Duration) client.ScheduleOptions {
	return client.ScheduleOptions{
		ID: id,
		Spec: client.ScheduleSpec{
			Intervals:    []client.ScheduleIntervalSpec{{Every: 24 * time.Hour, Offset: offset}},
			TimeZoneName: "UTC",
		},
		Action: &client.ScheduleWorkflowAction{
			ID:                 id,
			Workflow:           workflowName,
			TaskQueue:          FlowTaskQueue,
			WorkflowRunTimeout: 6 * time.Hour,
		},
		Overlap:        enums.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow:  24 * time.Hour,
		PauseOnFailure: true,
	}
}

func (i *ScheduleInstaller) ensureSchedule(ctx context.Context, options client.ScheduleOptions) error {
	_, err := i.client.Create(ctx, options)
	if err == nil {
		return nil
	}
	if !errors.Is(err, temporal.ErrScheduleAlreadyRunning) {
		return fmt.Errorf("create maintenance schedule %s: %w", options.ID, err)
	}
	description, err := i.client.GetHandle(ctx, options.ID).Describe(ctx)
	if err != nil {
		return fmt.Errorf("describe maintenance schedule %s: %w", options.ID, err)
	}
	if !sameCleanupSchedule(description, options) {
		return fmt.Errorf("%w: %s", ErrScheduleDrift, options.ID)
	}
	return nil
}

func sameCleanupSchedule(description *client.ScheduleDescription, options client.ScheduleOptions) bool {
	if description == nil || description.Schedule.Spec == nil || description.Schedule.Policy == nil {
		return false
	}
	action, ok := description.Schedule.Action.(*client.ScheduleWorkflowAction)
	if !ok || action.Workflow != options.Action.(*client.ScheduleWorkflowAction).Workflow {
		return false
	}
	wantAction := options.Action.(*client.ScheduleWorkflowAction)
	if action.ID != wantAction.ID || action.TaskQueue != wantAction.TaskQueue || action.WorkflowRunTimeout != wantAction.WorkflowRunTimeout {
		return false
	}
	spec := description.Schedule.Spec
	if len(spec.Intervals) != 1 || len(spec.Calendars) != 0 || len(spec.CronExpressions) != 0 || spec.TimeZoneName != options.Spec.TimeZoneName {
		return false
	}
	wantInterval := options.Spec.Intervals[0]
	if spec.Intervals[0].Every != wantInterval.Every || spec.Intervals[0].Offset != wantInterval.Offset {
		return false
	}
	policy := description.Schedule.Policy
	return policy.Overlap == options.Overlap &&
		policy.CatchupWindow == options.CatchupWindow &&
		policy.PauseOnFailure == options.PauseOnFailure
}
