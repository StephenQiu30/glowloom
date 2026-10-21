package workspace_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	maintenanceflow "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/temporal"
	workspaceflow "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/workflow"
)

func TestProjectRetentionRealScheduleAndWorker(t *testing.T) {
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if addr == "" || namespace == "" {
		t.Skip("set isolated Temporal address and namespace")
	}
	if namespace != "glowloom-canvas-management-20261021" {
		t.Fatal("requires dedicated retention test namespace")
	}
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	project := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,revision=2,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day' WHERE id=?`, project)
	temporalClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	defer temporalClient.Close()
	w := worker.New(temporalClient, "flow", worker.Options{})
	workspaceflow.RegisterProjectRetention(w, workspaceflow.NewProjectRetentionActivities(app.NewProjectRetention(db)))
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	prefix := "canvas-retention-test-" + uuid.NewString() + "-"
	installer := maintenanceflow.NewScheduleInstaller(temporalClient.ScheduleClient(), prefix)
	if err := installer.InstallProjectRetentionSchedule(ctx); err != nil {
		t.Fatal(err)
	}
	handle := temporalClient.ScheduleClient().GetHandle(ctx, prefix+maintenanceflow.ProjectRetentionScheduleID)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := handle.Delete(cleanup); err != nil {
			t.Error(err)
		}
	}()
	if err := installer.InstallProjectRetentionSchedule(ctx); err != nil {
		t.Fatal("schedule replay", err)
	}
	if err := handle.Trigger(ctx, client.ScheduleTriggerOptions{}); err != nil {
		t.Fatal("trigger isolated schedule", err)
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("scheduled worker did not complete retention")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
			var done bool
			if err := db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project_retention_job WHERE project_id=? AND deleted_revision=2 AND status='completed')`, project).Scan(&done).Error; err != nil {
				t.Fatal(err)
			}
			if done {
				return
			}
		}
	}
}
