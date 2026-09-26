package maintenance_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"

	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
)

func TestScheduleInstallerIsIdempotentAndDetectsDrift(t *testing.T) {
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(workflowClient.Close)
	prefix := "maintenance-installer-test-" + uuid.NewString() + "-"
	scheduleClient := workflowClient.ScheduleClient()
	for _, id := range []string{
		maintenanceflow.OutboxCleanupScheduleID,
		maintenanceflow.ProcessedEventCleanupScheduleID,
	} {
		handle := scheduleClient.GetHandle(ctx, prefix+id)
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			if err := handle.Delete(cleanupCtx); err != nil {
				t.Errorf("delete test schedule %s: %v", handle.GetID(), err)
			}
		})
	}
	installer := maintenanceflow.NewScheduleInstaller(scheduleClient, prefix)
	if err := installer.InstallCleanupSchedules(ctx); err != nil {
		t.Fatalf("install cleanup schedules: %v", err)
	}
	if err := installer.InstallCleanupSchedules(ctx); err != nil {
		t.Fatalf("repeat schedule installation: %v", err)
	}
	handle := scheduleClient.GetHandle(ctx, prefix+maintenanceflow.OutboxCleanupScheduleID)
	if err := handle.Pause(ctx, client.SchedulePauseOptions{Note: "maintenance test"}); err != nil {
		t.Fatalf("pause test schedule: %v", err)
	}
	if err := installer.InstallCleanupSchedules(ctx); err != nil {
		t.Fatalf("install with paused schedule: %v", err)
	}
	description, err := handle.Describe(ctx)
	if err != nil {
		t.Fatalf("describe paused schedule: %v", err)
	}
	if description.Schedule.State == nil || !description.Schedule.State.Paused {
		t.Fatal("schedule installation changed the paused state")
	}
	if err := handle.Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			schedule := input.Description.Schedule
			action := schedule.Action.(*client.ScheduleWorkflowAction)
			action.TaskQueue = "wrong-queue"
			return &client.ScheduleUpdate{Schedule: &schedule}, nil
		},
	}); err != nil {
		t.Fatalf("change test schedule task queue: %v", err)
	}
	if err := installer.InstallCleanupSchedules(ctx); !errors.Is(err, maintenanceflow.ErrScheduleDrift) {
		t.Fatalf("schedule drift error = %v", err)
	}
}
