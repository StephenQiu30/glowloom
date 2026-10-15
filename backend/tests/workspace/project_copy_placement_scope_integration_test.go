package workspace_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func TestProjectCopyPlacementPGCurrentActorPrivateScopeAndRevocation(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	verifyCopyPlacementScope(ctx, t, db, owner)
}

func TestWorkspaceCopyPlacementFixtureInRollbackTransaction(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer rollbackLifecycleTest(t, tx)
	verifyCopyPlacementScope(ctx, t, tx, tx)
}

func verifyCopyPlacementScope(ctx context.Context, t *testing.T, db, owner *gorm.DB) {
	t.Helper()
	actor, source := lifecycleActorProject(ctx, t, owner)
	other := insertWorkspaceActor(ctx, t, owner, actor.OrgID.String())
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "另一制片人的私人分类")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	service := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now)
	otherSource := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, other, "16:9", "realistic", nil))
	// A persisted root placement makes revision 1 current. Only the claimed
	// foreign folder differs, so public shape validation cannot mask isolation.
	lifecycleFixtureSQL(t, owner, `INSERT INTO workspace.project_folder_placement(org_id,owner_scope_id,actor_id,project_id,revision) VALUES(?,?,?,?,1)`, other.OrgID, other.ScopeID, other.ID, otherSource)
	foreignPlacement := copyInput(otherSource)
	foreignPlacement.Placement = &workspaceapp.CopyPlacementExpectation{ExpectedPlacementRevision: 1, FolderID: &f.ID, ExpectedFolderRevision: f.Revision}
	if err := foreignPlacement.Validate(); err != nil {
		t.Fatal("foreign folder fixture must reach admission", err)
	}
	if _, err := service.Create(ctx, other, foreignPlacement); !errors.Is(err, domain.ErrProjectFolderRevisionConflict) {
		t.Fatalf("another actor's folder bound to copy %v", err)
	}
	if _, err := service.Create(ctx, other, copyInput(source)); err == nil {
		t.Fatal("another owner's project copied")
	}
	var unauthorizedJobs int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_job WHERE actor_id=?`, other.ID).Scan(&unauthorizedJobs).Error; err != nil || unauthorizedJobs != 0 {
		t.Fatalf("rejected cross-owner copy left jobs %d %v", unauthorizedJobs, err)
	}
	input := copyInput(otherSource)
	job, err := service.Create(ctx, other, input)
	if err != nil {
		t.Fatal(err)
	}
	var target domain.FolderPlacement
	if err = db.Raw(`SELECT org_id,actor_id,project_id,folder_id,revision FROM workspace.project_folder_placement WHERE project_id=?`, job.TargetProjectID).Scan(&target).Error; err != nil || target.ActorID != other.ID || target.FolderID != nil || target.Revision != 1 {
		t.Fatalf("copy inherited another actor's classification %+v %v", target, err)
	}
	list, err := folders.List(ctx, other, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("private folder visible to other actor %+v %v", list, err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE identity."user" SET is_delete=true WHERE id=?`, other.ID)
	if _, err = service.Create(ctx, other, input); !errors.Is(err, identityapp.ErrSessionInvalid) {
		t.Fatalf("revoked current actor replayed old acceptance %v", err)
	}
	_, foreignSource := lifecycleActorProject(ctx, t, owner)
	if _, err = service.Create(ctx, actor, copyInput(foreignSource)); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("cross organization source copied %v", err)
	}
}

func TestProjectCopyPlacementPGMembershipRevisionBudgetRollsBack(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "目录成员版本预算")
	moveTestProject(ctx, t, folders, actor, source, f, 0)
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project_folder SET revision=? WHERE id=?`, math.MaxInt32, f.ID)
	in := copyInput(source)
	if _, err := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now).Create(ctx, actor, in); !errors.Is(err, domain.ErrProjectFolderRevisionConflict) {
		t.Fatalf("exhausted folder CAS must fail explicitly %v", err)
	}
	var jobs, placements int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_job WHERE actor_id=? AND idem_key=?`, actor.ID, in.IdempotencyKey).Scan(&jobs).Error; err != nil || jobs != 0 {
		t.Fatalf("revision overflow left copy job %d %v", jobs, err)
	}
	if err := db.Raw(`SELECT count(*) FROM workspace.project_folder_placement WHERE actor_id=?`, actor.ID).Scan(&placements).Error; err != nil || placements != 1 {
		t.Fatalf("revision overflow left target classification %d %v", placements, err)
	}
}
