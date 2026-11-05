package workspace_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	task "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
)

func TestProjectRetentionRealScheduleAndWorker(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	project := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,revision=2,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day' WHERE id=?`, project)
	taskfixture.Start(t, db, map[string]taskruntime.Handler{task.Retention: task.RetentionHandler(app.NewProjectRetention(db))})
	if err := taskruntime.Enqueue(ctx, db, task.Retention, uuid.NewString(), time.Now()); err != nil {
		t.Fatal(err)
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
