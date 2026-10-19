package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestProjectCreationRecoveryNeverCreatesAndReadsCurrentTarget(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	store := workspaceStore(db)
	query := workspaceapp.NewProjectCreationRecoveryQuery(store)
	key := uuid.New()
	before, err := query.Resolve(ctx, actor, key)
	if err != nil || before.State != "unrecorded" || before.ProjectID != nil || before.CanvasID != nil || before.ProjectStatus != nil {
		t.Fatalf("unrecorded: %+v %v", before, err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE owner_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("query wrote objects: %d %v", count, err)
	}
	created, err := workspaceapp.NewCreateProjectCommand(store, time.Now).ExecuteIdempotent(ctx, actor, key, workspaceapp.CreateProjectInput{Name: "未命名画布", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET name='修改后',status='archived',archived_at=now(),revision=2 WHERE id=?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	got, err := query.Resolve(ctx, actor, key)
	if err != nil || got.State != "created" || got.ProjectID == nil || *got.ProjectID != created.ID || got.CanvasID == nil || *got.CanvasID != created.CanvasID || got.ProjectStatus == nil || *got.ProjectStatus != "archived" {
		t.Fatalf("current recovery: %+v %v", got, err)
	}
	other := insertWorkspaceActor(ctx, t, db, actor.OrgID.String())
	private, err := query.Resolve(ctx, other, key)
	if err != nil || private.State != "unrecorded" || private.ProjectID != nil {
		t.Fatalf("other actor receipt leaked: %+v %v", private, err)
	}
	if err := db.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=now(),purge_after=now()+interval '30 days' WHERE id=?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Resolve(ctx, actor, key); !errors.Is(err, workspaceapp.ErrCreationTargetUnavailable) {
		t.Fatalf("deleted target: %v", err)
	}
}
