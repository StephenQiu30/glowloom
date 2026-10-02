package workspace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type fixedProjectWork struct {
	blocked bool
	err     error
}

func (g fixedProjectWork) HasInflightWork(_ context.Context, _ identityapp.Principal, _ uuid.UUID) (bool, error) {
	return g.blocked, g.err
}

func lifecycleStore(db *gorm.DB, work workspaceapp.ProjectWorkGuard) *pgworkspace.Store {
	return pgworkspace.NewStoreWithProjectWorkGuard(db, func(*gorm.DB) workspaceapp.ProjectWorkGuard { return work })
}

func projectChange(id uuid.UUID, revision int64, action string) workspaceapp.ProjectChangeInput {
	return workspaceapp.ProjectChangeInput{Action: action, IdempotencyKey: uuid.New(), Patch: workspaceapp.UpdateProjectInput{ProjectID: id, ExpectedRevision: revision, RequestID: uuid.NewString()}}
}

func TestProjectLifecyclePersistsReplayRecoveryAndScope(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	id := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor.OrgID.String(), "16:9", "realistic", nil))
	now := time.Now().UTC()
	service := workspaceapp.NewProjectLifecycle(lifecycleStore(db, fixedProjectWork{}), func() time.Time { return now })
	read, err := service.Get(ctx, actor, id)
	if err != nil || read.Project.ID != id || read.DefaultModels == nil {
		t.Fatalf("get %+v err=%v", read, err)
	}
	patch := projectChange(id, 1, "patch")
	name := "  完整生命周期  "
	patch.Patch.Name = &name
	saved, err := service.Change(ctx, actor, patch)
	if err != nil || saved.Project.Name != "完整生命周期" || saved.Project.Revision != 2 {
		t.Fatalf("patch %+v err=%v", saved, err)
	}
	archive := projectChange(id, 2, "archive")
	archived, err := service.Change(ctx, actor, archive)
	if err != nil || archived.Project.Status != "archived" || archived.Project.ArchivedAt == nil {
		t.Fatalf("archive %+v err=%v", archived, err)
	}
	replay, err := workspaceapp.NewProjectLifecycle(lifecycleStore(db, fixedProjectWork{blocked: true}), time.Now).Change(ctx, actor, archive)
	if err != nil || replay.Project.Revision != 3 || !replay.Project.UpdateTime.Equal(archived.Project.UpdateTime) {
		t.Fatalf("replay %+v err=%v", replay, err)
	}
	changed := archive
	changed.Action = "unarchive"
	if _, err := service.Change(ctx, actor, changed); !errors.Is(err, workspaceapp.ErrIdempotencyConflict) {
		t.Fatalf("same key different action %v", err)
	}
	deleted, err := service.Change(ctx, actor, projectChange(id, 3, "delete"))
	if err != nil || !deleted.Project.IsDelete || deleted.Project.PurgeAfter == nil || deleted.Project.Status != "archived" {
		t.Fatalf("delete %+v err=%v", deleted, err)
	}
	if _, err := service.Get(ctx, actor, id); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("deleted read %v", err)
	}
	restored, err := service.Change(ctx, actor, projectChange(id, 4, "restore"))
	if err != nil || restored.Project.IsDelete || restored.Project.Status != "archived" || restored.Project.Revision != 5 {
		t.Fatalf("restore %+v err=%v", restored, err)
	}
	active, err := service.Change(ctx, actor, projectChange(id, 5, "unarchive"))
	if err != nil || active.Project.Status != "active" || active.Project.ArchivedAt != nil {
		t.Fatalf("unarchive %+v err=%v", active, err)
	}
	nop := projectChange(id, 6, "patch")
	nop.Patch.Name = &saved.Project.Name
	if p, err := service.Change(ctx, actor, nop); err != nil || p.Project.Revision != 6 {
		t.Fatalf("no-op %+v err=%v", p, err)
	}
	other := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	if _, err := service.Get(ctx, other, id); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("crossorg %v", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(ctx, actor, archive); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked replay %v", err)
	}
}

func TestProjectLifecycleRejectsMissingWorkEvidenceAndStaleCAS(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	id := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor.OrgID.String(), "16:9", "realistic", nil))
	input := projectChange(id, 1, "delete")
	service := workspaceapp.NewProjectLifecycle(pgworkspace.NewStore(db), time.Now)
	if _, err := service.Change(ctx, actor, input); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatalf("missing guard %v", err)
	}
	service = workspaceapp.NewProjectLifecycle(lifecycleStore(db, fixedProjectWork{blocked: true}), time.Now)
	if _, err := service.Change(ctx, actor, input); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("inflight %v", err)
	}
	service = workspaceapp.NewProjectLifecycle(lifecycleStore(db, fixedProjectWork{}), time.Now)
	if _, err := service.Change(ctx, actor, projectChange(id, 2, "archive")); !errors.Is(err, domain.ErrProjectRevisionConflict) {
		t.Fatalf("CAS %v", err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project_change_command WHERE actor_id=?`, actor.ID).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rejected mutation recorded success")
	}
}
