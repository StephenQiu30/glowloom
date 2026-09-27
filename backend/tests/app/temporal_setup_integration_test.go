package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
)

func TestTemporalSetupInstallsCleanupSchedules(t *testing.T) {
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
	prefix := "setup-test-" + uuid.NewString() + "-"
	for _, id := range []string{maintenanceflow.OutboxCleanupScheduleID, maintenanceflow.ProcessedEventCleanupScheduleID} {
		handle := workflowClient.ScheduleClient().GetHandle(ctx, prefix+id)
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			if err := handle.Delete(cleanupCtx); err != nil {
				t.Errorf("delete test schedule %s: %v", handle.GetID(), err)
			}
		})
	}
	cfg := config.Config{TemporalAddr: addr, TemporalNamespace: namespace}
	if err := app.InstallCleanupSchedules(ctx, cfg, zap.NewNop(), prefix); err != nil {
		t.Fatalf("install cleanup schedules: %v", err)
	}
	if err := app.InstallCleanupSchedules(ctx, cfg, zap.NewNop(), prefix); err != nil {
		t.Fatalf("repeat installation: %v", err)
	}
	for _, id := range []string{maintenanceflow.OutboxCleanupScheduleID, maintenanceflow.ProcessedEventCleanupScheduleID} {
		description, err := workflowClient.ScheduleClient().GetHandle(ctx, prefix+id).Describe(ctx)
		if err != nil || description == nil {
			t.Fatalf("describe schedule %s: %v", id, err)
		}
		action, ok := description.Schedule.Action.(*client.ScheduleWorkflowAction)
		if !ok || action.TaskQueue != maintenanceflow.FlowTaskQueue {
			t.Fatalf("schedule %s action = %+v", id, description.Schedule.Action)
		}
	}
}
