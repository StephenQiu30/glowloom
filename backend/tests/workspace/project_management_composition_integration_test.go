package workspace_test

import (
	"errors"
	"testing"
	"time"

	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Use the installed owner list, rather than the smaller historical test guard.
func TestProjectManagementComposedOwnersRestoreRecycledProject(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, id := lifecycleActorProject(ctx, t, db)
	service := workspaceapp.NewProjectLifecycle(workspacewire.ProvideWorkspaceLifecycleStore(db), time.Now)
	archived, err := service.Change(ctx, actor, projectChange(id, 1, "archive"))
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := service.Change(ctx, actor, projectChange(id, archived.Project.Revision, "delete"))
	if err != nil {
		t.Fatal(err)
	}
	other := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	if _, err := service.Change(ctx, other, projectChange(id, deleted.Project.Revision, "restore")); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("cross scope restore: %v", err)
	}
	restored, err := service.Change(ctx, actor, projectChange(id, deleted.Project.Revision, "restore"))
	if err != nil || restored.Project.IsDelete || restored.Project.Status != "archived" {
		t.Fatalf("restore %+v: %v", restored, err)
	}
}
