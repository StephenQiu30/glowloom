package workspace_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCopyPlacementPGUnremovedObjectKeepsTargetAndDirectoryFenced(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	asset := uuid.New()
	// This test exercises real persistence of a pending transfer intent, not object delivery.
	lifecycleFixtureSQL(t, owner, `INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,moderation_status,sha256,width,height)VALUES(?,?,'image','upload','ready',?,'image/png',12,'passed',repeat('a',64),2,2)`, asset, source, "projects/"+source.String()+"/image/2026/10/"+asset.String()+".png")
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "待清理对象保持屏障")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	store := projectCopyStore(db)
	service := workspaceapp.NewProjectCopyService(store, time.Now)
	job, err := service.Create(ctx, actor, copyInput(source))
	if err != nil || job.Manifest.Assets != 1 {
		t.Fatalf("real owner snapshot %+v %v", job, err)
	}
	if _, err = service.Change(ctx, actor, job.ID, "cancel", job.Revision, uuid.New(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	_, err = store.Claim(ctx, actor, job.ID, worker, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Find(ctx, actor, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.FinishCancelled(ctx, actor, job.ID, worker); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatalf("pending owned object accepted as absent %v", err)
	}
	current, err := store.Find(ctx, actor, job.ID)
	if err != nil || !reflect.DeepEqual(current, claimed) {
		t.Fatalf("failed cleanup changed durable job %+v %v", current, err)
	}
	var target struct {
		IsDelete bool
		Revision int64
	}
	if err = db.Raw(`SELECT is_delete,revision FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&target).Error; err != nil || target.IsDelete || target.Revision != 1 {
		t.Fatalf("pending object retired target %+v %v", target, err)
	}
	in := folderCommand("recycle")
	in.FolderID, in.ExpectedRevision = f.ID, f.Revision+1
	if _, err = folders.Change(ctx, actor, in); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("pending cleanup failed to protect directory %v", err)
	}
}

func TestProjectCopyPlacementPGFailedThenCancelledCleanupKeepsHistoricalReceipt(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "失败清理后的目录")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	store := projectCopyStore(db)
	service := workspaceapp.NewProjectCopyService(store, time.Now)
	input := copyInput(source)
	job, err := service.Create(ctx, actor, input)
	if err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	if _, err = store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	failed, err := store.Fail(ctx, actor, job.ID, worker, "object_mismatch", false, false)
	if err != nil {
		t.Fatal(err)
	}
	in := folderCommand("recycle")
	in.FolderID, in.ExpectedRevision = f.ID, f.Revision+1
	if _, err = folders.Change(ctx, actor, in); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("failed unpublished copy no longer protected %v", err)
	}
	if _, err = service.Change(ctx, actor, job.ID, "cancel", failed.Revision, uuid.New(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	cleanupWorker := uuid.New()
	if _, err = store.Claim(ctx, actor, job.ID, cleanupWorker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.FinishCancelled(ctx, actor, job.ID, cleanupWorker)
	if err != nil {
		t.Fatal(err)
	}
	var before, after struct {
		DeleteTime, PurgeAfter time.Time
		Revision               int64
	}
	if err = db.Raw(`SELECT delete_time,purge_after,revision FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	var eventsBefore, eventsAfter int64
	if err = db.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, job.ID.String()).Scan(&eventsBefore).Error; err != nil {
		t.Fatal(err)
	}
	again, err := store.FinishCancelled(ctx, actor, job.ID, cleanupWorker)
	if err != nil || !reflect.DeepEqual(again, cancelled) {
		t.Fatalf("cancelled repeat changed job %+v %v", again, err)
	}
	if err = db.Raw(`SELECT delete_time,purge_after,revision FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&after).Error; err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("cancelled repeat moved tombstone %+v %v", after, err)
	}
	if err = db.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, job.ID.String()).Scan(&eventsAfter).Error; err != nil || eventsBefore != eventsAfter {
		t.Fatalf("cancelled repeat created outbox %d/%d %v", eventsBefore, eventsAfter, err)
	}
	if _, err = folders.Change(ctx, actor, in); err != nil {
		t.Fatalf("actual completed cleanup stranded failed target %v", err)
	}
	replay, err := service.Create(ctx, actor, input)
	if err != nil || !reflect.DeepEqual(replay, job) {
		t.Fatalf("cancelled historical admission changed %+v %v", replay, err)
	}
}
