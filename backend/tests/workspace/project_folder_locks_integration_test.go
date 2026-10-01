package workspace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectFolderPGAssignmentAndRecycleHavePhysicalFence(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "实际成员锁")
	f = *moveTestProject(ctx, t, service, actor, project, f, 0).Folder
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	admission := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, admission)
	move := folderCommand("move")
	move.ProjectID = project
	move.ExpectedProjectRevision = 1
	move.ExpectedPlacementRevision = 1
	if _, err := workspaceapp.NewProjectFolders(folderStore(admission), time.Now).Change(owned, actor, move); err != nil {
		t.Fatal(err)
	}
	recycleTx := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, recycleTx)
	blocker, blocked := projectTransactionPID(t, admission), projectTransactionPID(t, recycleTx)
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = f.Revision
	done := make(chan error, 1)
	finished := false
	go func() {
		_, err := workspaceapp.NewProjectFolders(folderStore(recycleTx), time.Now).Change(owned, actor, recycle)
		done <- err
	}()
	defer func() {
		if !finished {
			cancel()
			<-done
		}
	}()
	waitProjectLock(owned, t, db, blocker, blocked)
	if err := admission.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if !errors.Is(err, domain.ErrProjectFolderRevisionConflict) {
			t.Fatalf("recycle bypassed membership CAS %v", err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	var deleted bool
	if err := db.Raw(`SELECT is_delete FROM workspace.project WHERE id=?`, project).Scan(&deleted).Error; err != nil || deleted {
		t.Fatal("moved member was recycled", err)
	}
}

type pausedFolderGuard struct {
	owner   workspaceapp.ProjectWorkGuard
	entered chan struct{}
	release chan struct{}
}

func (g pausedFolderGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	close(g.entered)
	select {
	case <-g.release:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return g.owner.HasInflightWork(ctx, actor, project)
}
func TestProjectFolderPGAlreadyDeletedMemberRestoreIsSerialized(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "保留已回收成员锁")
	f = *moveTestProject(ctx, t, service, actor, project, f, 0).Folder
	lifecycle := workspaceapp.NewProjectLifecycle(workspacepg.NewStoreWithProjectWorkGuard(db, folderWork), time.Now)
	if _, err := lifecycle.Change(ctx, actor, projectChange(project, 1, "delete")); err != nil {
		t.Fatal(err)
	}
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	recycleTx := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, recycleTx)
	restoreTx := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, restoreTx)
	entered, release := make(chan struct{}), make(chan struct{})
	guardFactory := func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return pausedFolderGuard{folderWork(tx), entered, release}
	}
	recycler := workspaceapp.NewProjectFolders(workspacepg.NewFolderStore(recycleTx, guardFactory, nil), time.Now)
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = f.Revision
	recycleDone := make(chan error, 1)
	restoreDone := make(chan error, 1)
	recycleFinished, restoreStarted, restoreFinished := false, false, false
	go func() { _, err := recycler.Change(owned, actor, recycle); recycleDone <- err }()
	defer func() {
		cancel()
		if !recycleFinished {
			<-recycleDone
		}
		if restoreStarted && !restoreFinished {
			<-restoreDone
		}
	}()
	select {
	case <-entered:
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	blocker, blocked := projectTransactionPID(t, recycleTx), projectTransactionPID(t, restoreTx)
	restoreStarted = true
	go func() {
		_, err := workspaceapp.NewProjectLifecycle(workspacepg.NewStoreWithProjectWorkGuard(restoreTx, folderWork), time.Now).Change(owned, actor, projectChange(project, 2, "restore"))
		restoreDone <- err
	}()
	waitProjectLock(owned, t, db, blocker, blocked)
	close(release)
	select {
	case err := <-recycleDone:
		recycleFinished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	if err := recycleTx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-restoreDone:
		restoreFinished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	if err := restoreTx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	root := uuid.Nil
	page, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &root, Limit: 50})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].ID != project || page.Projects[0].PlacementRevision != 2 {
		t.Fatalf("serialized restore root %+v %v", page, err)
	}
}

func TestProjectFolderPGCoverReferenceLockSurvivesUntilCommit(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	asset := uuid.New()
	lifecycleFixtureSQL(t, owner, `INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,moderation_status,sha256)VALUES(?,?,'image','upload','ready',?,'image/png',12,'passed',repeat('a',64))`, asset, project, "projects/"+project.String()+"/image/2026/10/"+asset.String()+".png")
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	save := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, save)
	withdraw := owner.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, withdraw)
	in := folderCommand("create")
	name := "共享媒体锁"
	in.Name = &name
	in.SetCover = true
	in.Cover = &domain.FolderCover{ProjectID: project, AssetID: asset}
	if _, err := workspaceapp.NewProjectFolders(folderStore(save), time.Now).Change(owned, actor, in); err != nil {
		t.Fatal(err)
	}
	blocker, blocked := projectTransactionPID(t, save), projectTransactionPID(t, withdraw)
	done := make(chan error, 1)
	finished := false
	go func() {
		done <- withdraw.Exec(`UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, asset).Error
	}()
	defer func() {
		if !finished {
			cancel()
			<-done
		}
	}()
	waitProjectLock(owned, t, db, blocker, blocked)
	if err := save.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	if err := withdraw.Commit().Error; err != nil {
		t.Fatal(err)
	}
	list, err := workspaceapp.NewProjectFolders(folderStore(db), time.Now).List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || len(list.Items) != 1 || !list.Items[0].CoverUnavailable || list.Items[0].Folder.Cover != nil {
		t.Fatalf("withdrawn cover %+v %v", list, err)
	}
}
