package workspace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCopyPlacementPGAdmissionSerializesWholeFolderRecycle(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "复制与回收实际共享锁")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	admission := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, admission)
	job, err := workspaceapp.NewProjectCopyService(projectCopyStore(admission), time.Now).Create(owned, actor, copyInput(source))
	if err != nil {
		t.Fatal(err)
	}
	recycler := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, recycler)
	blocker, blocked := projectTransactionPID(t, admission), projectTransactionPID(t, recycler)
	in := folderCommand("recycle")
	in.FolderID, in.ExpectedRevision = f.ID, f.Revision
	done := make(chan error, 1)
	finished := false
	go func() {
		_, err := workspaceapp.NewProjectFolders(folderStore(recycler), time.Now).Change(owned, actor, in)
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
			t.Fatalf("directory bypassed admitted member CAS %v", err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	if err := recycler.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	in.IdempotencyKey, in.RequestID, in.ExpectedRevision = uuid.New(), uuid.NewString(), f.Revision+1
	if _, err := folders.Change(ctx, actor, in); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("admitted copying member bypassed owning guard %v", err)
	}
	var active int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE id IN (?,?) AND NOT is_delete`, source, job.TargetProjectID).Scan(&active).Error; err != nil || active != 2 {
		t.Fatalf("rejected recycle partially deleted members %d %v", active, err)
	}
}

func TestProjectCopyPlacementPGPublicationRejectsChangedTargetClassification(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	store := projectCopyStore(db)
	job, err := workspaceapp.NewProjectCopyService(store, time.Now).Create(ctx, actor, copyInput(source))
	if err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	if _, err = store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteMedia(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteCanvases(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	// Only the fixture owner can violate the immutable copying target's placement.
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project_folder_placement SET revision=2 WHERE actor_id=? AND project_id=?`, actor.ID, job.TargetProjectID)
	if _, err = store.Publish(ctx, actor, job.ID, worker); !errors.Is(err, domain.ErrInvalidProjectCopy) {
		t.Fatalf("modified target placement published %v", err)
	}
	var status string
	if err = db.Raw(`SELECT status FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&status).Error; err != nil || status != "copying" {
		t.Fatalf("partial publication %s %v", status, err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project_folder_placement SET revision=1 WHERE actor_id=? AND project_id=?`, actor.ID, job.TargetProjectID)
	if _, err = store.Publish(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
}

func TestProjectCopyPlacementPGCleanupAndDirectoryRecycleHaveNoReverseLock(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "清理与整目录回收")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	store := projectCopyStore(db)
	service := workspaceapp.NewProjectCopyService(store, time.Now)
	job, err := service.Create(ctx, actor, copyInput(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Change(ctx, actor, job.ID, "cancel", job.Revision, uuid.New(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	if _, err = store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	cleanup := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, cleanup)
	if _, err = projectCopyStore(cleanup).FinishCancelled(owned, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	recycler := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, recycler)
	blocker, blocked := projectTransactionPID(t, cleanup), projectTransactionPID(t, recycler)
	in := folderCommand("recycle")
	in.FolderID, in.ExpectedRevision = f.ID, f.Revision+1
	done := make(chan error, 1)
	finished := false
	go func() {
		_, err := workspaceapp.NewProjectFolders(folderStore(recycler), time.Now).Change(owned, actor, in)
		done <- err
	}()
	defer func() {
		if !finished {
			cancel()
			<-done
		}
	}()
	waitProjectLock(owned, t, db, blocker, blocked)
	if err = cleanup.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		finished = true
		if err != nil {
			t.Fatalf("cleanup retirement stranded/deadlocked directory %v", err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	if err = recycler.Commit().Error; err != nil {
		t.Fatal(err)
	}
	var placements int64
	if err = db.Raw(`SELECT count(*) FROM workspace.project_folder_placement WHERE actor_id=? AND project_id IN (?,?) AND folder_id IS NULL AND revision=2`, actor.ID, source, job.TargetProjectID).Scan(&placements).Error; err != nil || placements != 2 {
		t.Fatalf("serialized recycle did not clear complete membership %d %v", placements, err)
	}
}
