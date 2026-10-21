package app_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	maintenanceflow "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/temporal"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestTemporalSetupInstallsImplementedSchedules(t *testing.T) {
	addr := os.Getenv("GLOWLOOM_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("GLOWLOOM_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set GLOWLOOM_TEST_TEMPORAL_ADDR and GLOWLOOM_TEST_TEMPORAL_NAMESPACE")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatalf("connect Temporal: %v", err)
	}
	t.Cleanup(workflowClient.Close)
	prefix := "setup-test-" + uuid.NewString() + "-"
	for _, id := range []string{maintenanceflow.OutboxCleanupScheduleID, maintenanceflow.ProcessedEventCleanupScheduleID, operationflow.QuoteExpiryScheduleID} {
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
	if err := app.InstallQuoteExpirySchedule(ctx, cfg, zap.NewNop(), prefix); err != nil {
		t.Fatalf("install quote expiry schedule: %v", err)
	}
	if err := app.InstallQuoteExpirySchedule(ctx, cfg, zap.NewNop(), prefix); err != nil {
		t.Fatalf("repeat quote expiry installation: %v", err)
	}
	for _, id := range []string{maintenanceflow.OutboxCleanupScheduleID, maintenanceflow.ProcessedEventCleanupScheduleID, operationflow.QuoteExpiryScheduleID} {
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

func TestTemporalSetupProjectRetentionOnlyDoesNotEnableOtherMaintenance(t *testing.T) {
	addr, namespace := os.Getenv("GLOWLOOM_TEST_TEMPORAL_ADDR"), os.Getenv("GLOWLOOM_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set isolated Temporal address and namespace")
	}
	if namespace != "glowloom-canvas-management-20261021" {
		t.Fatal("requires dedicated canvas-management namespace")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	c, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	prefix := "retention-only-test-" + uuid.NewString() + "-"
	ids := []string{maintenanceflow.ProjectRetentionScheduleID, maintenanceflow.OutboxCleanupScheduleID, maintenanceflow.ProcessedEventCleanupScheduleID, operationflow.QuoteExpiryScheduleID}
	defer func() {
		for _, id := range ids {
			_, err := c.ScheduleClient().GetHandle(ctx, prefix+id).Describe(ctx)
			if err == nil {
				if err := c.ScheduleClient().GetHandle(ctx, prefix+id).Delete(ctx); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	cfg := config.Config{TemporalAddr: addr, TemporalNamespace: namespace}
	for range 2 {
		if err := app.SetupTemporalSchedules(ctx, cfg, zap.NewNop(), prefix, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.ScheduleClient().GetHandle(ctx, prefix+ids[0]).Describe(ctx); err != nil {
		t.Fatal("project retention not installed", err)
	}
	for _, id := range ids[1:] {
		_, err := c.ScheduleClient().GetHandle(ctx, prefix+id).Describe(ctx)
		var missing *serviceerror.NotFound
		if !errors.As(err, &missing) {
			t.Fatalf("unrequested maintenance %s: expected NotFound, got %v", id, err)
		}
	}
}
