package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

// QuoteExpiryScheduleID is the five-minute unconfirmed quote sweep.
const QuoteExpiryScheduleID = "quote-expiry"

// ErrQuoteExpiryScheduleDrift means an installed schedule differs from its contract.
var ErrQuoteExpiryScheduleDrift = errors.New("quote expiry schedule differs from declared configuration")

// QuoteExpiryScheduleInstaller owns installation of the operation schedule.
type QuoteExpiryScheduleInstaller struct {
	client client.ScheduleClient
	prefix string
}

// NewQuoteExpiryScheduleInstaller injects the Temporal schedule client.
func NewQuoteExpiryScheduleInstaller(scheduleClient client.ScheduleClient, prefix string) *QuoteExpiryScheduleInstaller {
	return &QuoteExpiryScheduleInstaller{client: scheduleClient, prefix: prefix}
}

// Install creates an idempotent schedule without changing an existing pause state.
func (i *QuoteExpiryScheduleInstaller) Install(ctx context.Context) error {
	if i == nil || i.client == nil {
		return ErrQuoteExpiryScheduleDrift
	}
	id := i.prefix + QuoteExpiryScheduleID
	options := client.ScheduleOptions{
		ID: id,
		Spec: client.ScheduleSpec{
			Intervals:    []client.ScheduleIntervalSpec{{Every: 5 * time.Minute}},
			TimeZoneName: "UTC",
		},
		Action: &client.ScheduleWorkflowAction{
			ID: id, Workflow: "QuoteExpiryWorkflow", TaskQueue: "flow",
			WorkflowRunTimeout: time.Hour,
		},
		Overlap:        enums.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow:  24 * time.Hour,
		PauseOnFailure: true,
	}
	_, err := i.client.Create(ctx, options)
	if err == nil {
		return nil
	}
	if !errors.Is(err, temporal.ErrScheduleAlreadyRunning) {
		return fmt.Errorf("create quote expiry schedule %s: %w", id, err)
	}
	description, err := i.client.GetHandle(ctx, id).Describe(ctx)
	if err != nil {
		return fmt.Errorf("describe quote expiry schedule %s: %w", id, err)
	}
	if !sameQuoteExpirySchedule(description, options) {
		return fmt.Errorf("%w: %s", ErrQuoteExpiryScheduleDrift, id)
	}
	return nil
}

func sameQuoteExpirySchedule(description *client.ScheduleDescription, options client.ScheduleOptions) bool {
	if description == nil || description.Schedule.Spec == nil || description.Schedule.Policy == nil {
		return false
	}
	action, ok := description.Schedule.Action.(*client.ScheduleWorkflowAction)
	want := options.Action.(*client.ScheduleWorkflowAction)
	if !ok || action.ID != want.ID || action.Workflow != want.Workflow ||
		action.TaskQueue != want.TaskQueue || action.WorkflowRunTimeout != want.WorkflowRunTimeout {
		return false
	}
	spec := description.Schedule.Spec
	if len(spec.Intervals) != 1 || len(spec.Calendars) != 0 || len(spec.CronExpressions) != 0 ||
		spec.TimeZoneName != options.Spec.TimeZoneName ||
		spec.Intervals[0].Every != options.Spec.Intervals[0].Every || spec.Intervals[0].Offset != 0 {
		return false
	}
	policy := description.Schedule.Policy
	return policy.Overlap == options.Overlap && policy.CatchupWindow == options.CatchupWindow &&
		policy.PauseOnFailure == options.PauseOnFailure
}
