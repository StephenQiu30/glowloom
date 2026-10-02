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

func workspaceReferenceFixture(t *testing.T) (context.Context, *gorm.DB, identityapp.Principal, uuid.UUID) {
	t.Helper()
	ctx, db := workspaceMigrationDB(t)
	var name string
	if db.Raw(`SELECT current_database()`).Scan(&name).Error != nil || name != "lanverse_reference" {
		t.Fatal("isolated reference database required")
	}
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor.OrgID.String(), "16:9", "realistic", nil))
	return ctx, db, actor, project
}
func workspaceReferenceCheck(t *testing.T, db *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	t.Helper()
	var found bool
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var err error
		found, err = pgworkspace.NewMediaReferenceGuard(tx).HasMediaReferences(t.Context(), actor, project, asset)
		return err
	})
	return found, err
}
func TestWorkspaceMediaReferenceGuardRetainsOriginalCoversPresetAndDirectoryHistory(t *testing.T) {
	ctx, db, actor, project := workspaceReferenceFixture(t)
	assets := []uuid.UUID{seedCoverAsset(t, db, project, "image"), seedCoverAsset(t, db, project, "image"), seedCoverAsset(t, db, project, "image"), seedCoverAsset(t, db, project, "image"), seedCoverAsset(t, db, project, "image")}
	service := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	if _, err := service.Change(ctx, actor, coverChange(project, 1, &assets[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(ctx, actor, coverChange(project, 2, &assets[1])); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspace.style_preset(id,org_id,project_id,name,style_type,reference_asset_ids,is_delete)VALUES(?,?,?,'保留风格','realistic',ARRAY[?::uuid],true)`, uuid.New(), actor.OrgID, project, assets[2]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspace.project_folder(id,org_id,actor_id,name,cover_project_id,cover_asset_id,is_delete,delete_time)VALUES(?,?,?,'已回收目录',?,?,true,now())`, uuid.New(), actor.OrgID, actor.ID, project, assets[3]).Error; err != nil {
		t.Fatal(err)
	}
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	name := "历史目录"
	create := folderCommand("create")
	create.Name = &name
	create.SetCover = true
	create.Cover = &domain.FolderCover{ProjectID: project, AssetID: assets[4]}
	saved, err := folders.Change(ctx, actor, create)
	if err != nil || saved.Folder == nil {
		t.Fatal("create historical folder", err)
	}
	clearCover := folderCommand("patch")
	clearCover.FolderID = saved.Folder.ID
	clearCover.ExpectedRevision = saved.Folder.Revision
	clearCover.SetCover = true
	if _, err := folders.Change(ctx, actor, clearCover); err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		if found, err := workspaceReferenceCheck(t, db, actor, project, asset); err != nil || !found {
			t.Fatal("lost owning history reference", found, err)
		}
	}
	if found, err := workspaceReferenceCheck(t, db, actor, project, uuid.New()); err != nil || found {
		t.Fatal("unreferenced own asset", found, err)
	}
}
func TestWorkspaceMediaReferenceGuardRequiresCurrentScopeAndCallerTransaction(t *testing.T) {
	_, db, actor, project := workspaceReferenceFixture(t)
	if _, err := pgworkspace.NewMediaReferenceGuard(db).HasMediaReferences(t.Context(), actor, project, uuid.New()); err == nil {
		t.Fatal("pool accepted")
	}
	foreign := insertWorkspaceActor(t.Context(), t, db, insertWorkspaceOrganization(t.Context(), t, db))
	if _, err := workspaceReferenceCheck(t, db, foreign, project, uuid.New()); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatal("foreign owning scope", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := workspaceReferenceCheck(t, db, actor, project, uuid.New()); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("disabled current actor", err)
	}
}
func TestWorkspaceMediaReferenceGuardCorruptPermanentHistoryFailsClosed(t *testing.T) {
	_, db, actor, project := workspaceReferenceFixture(t)
	if err := db.Exec(`INSERT INTO workspace.project_change_command(id,org_id,project_id,actor_id,idem_key,action,request_sha256,status_code,response_body)VALUES(?,?,?,?,?,'patch',repeat('a',64),200,'{"undeclared_private_asset":"not-proof"}'::jsonb)`, uuid.New(), actor.OrgID, project, actor.ID, uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if found, err := workspaceReferenceCheck(t, db, actor, project, uuid.New()); err == nil {
		t.Fatal("corrupt history became absence", found)
	}
}
