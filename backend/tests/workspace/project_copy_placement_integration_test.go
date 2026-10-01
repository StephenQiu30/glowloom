package workspace_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCopyPlacementPGAtomicFreezePublicationAndReplay(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "复制到当前个人目录")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	store := projectCopyStore(db)
	service := workspaceapp.NewProjectCopyService(store, time.Now)
	input := copyInput(source)
	input.Placement = &workspaceapp.CopyPlacementExpectation{ExpectedPlacementRevision: 1, FolderID: &f.ID, ExpectedFolderRevision: f.Revision}
	job, err := service.Create(ctx, actor, input)
	if err != nil {
		t.Fatal(err)
	}
	var placement domain.FolderPlacement
	if err = db.Raw(`SELECT org_id,actor_id,project_id,folder_id,revision FROM workspace.project_folder_placement WHERE actor_id=? AND project_id=?`, actor.ID, job.TargetProjectID).Scan(&placement).Error; err != nil || placement.Validate() != nil || placement.FolderID == nil || *placement.FolderID != f.ID || placement.Revision != 1 {
		t.Fatalf("copy target placement %+v %v", placement, err)
	}
	var rev int64
	if err = db.Raw(`SELECT revision FROM workspace.project_folder WHERE id=?`, f.ID).Scan(&rev).Error; err != nil || rev != f.Revision+1 {
		t.Fatalf("member CAS %d %v", rev, err)
	}
	f.Revision = rev
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = f.Revision
	if _, err = folders.Change(ctx, actor, recycle); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("copying target/source did not protect directory %v", err)
	}
	rename := folderCommand("patch")
	rename.FolderID = f.ID
	rename.ExpectedRevision = f.Revision
	name := "后续更名"
	rename.Name = &name
	out, err := folders.Change(ctx, actor, rename)
	if err != nil {
		t.Fatal(err)
	}
	f = *out.Folder
	// Classification is independent from frozen project content. Source movement must not alter target placement.
	move := folderCommand("move")
	move.ProjectID = source
	move.ExpectedProjectRevision = 1
	move.ExpectedPlacementRevision = 1
	if _, err = folders.Change(ctx, actor, move); err != nil {
		t.Fatal(err)
	}
	replay, err := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now).Create(ctx, actor, input)
	if err != nil || !reflect.DeepEqual(job, replay) {
		t.Fatalf("later classification invalidated receipt %+v %v", replay, err)
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
	if _, err = store.Publish(ctx, actor, job.ID, worker); err != nil {
		t.Fatalf("source folder change invalidated frozen target %v", err)
	}
	page, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &f.ID, Limit: 50})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].ID != job.TargetProjectID {
		t.Fatalf("published target location %+v %v", page, err)
	}
	// Even folder deletion after completed copy cannot invalidate the original admission response.
	list, err := folders.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	recycle.ExpectedRevision = list.Items[0].Folder.Revision
	if _, err = folders.Change(ctx, actor, recycle); err != nil {
		t.Fatal(err)
	}
	replay, err = service.Create(ctx, actor, input)
	if err != nil || !reflect.DeepEqual(job, replay) {
		t.Fatalf("deleted folder broke original receipt %+v %v", replay, err)
	}
	changed := input
	expect := *input.Placement
	expect.ExpectedFolderRevision++
	changed.Placement = &expect
	if _, err = service.Create(ctx, actor, changed); !errors.Is(err, workspaceapp.ErrIdempotencyConflict) {
		t.Fatalf("same key changed placement accepted %v", err)
	}
	var recorded struct{ WorkspaceSnapshot json.RawMessage }
	if err = db.Raw(`SELECT workspace_snapshot FROM workspace.project_copy_job WHERE id=?`, job.ID).Scan(&recorded).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]json.RawMessage
	if json.Unmarshal(recorded.WorkspaceSnapshot, &snapshot) != nil || len(snapshot["Placement"]) == 0 {
		t.Fatal("classification missing from immutable workspace snapshot")
	}
}

func TestProjectCopyPlacementPGCancelledTargetDoesNotStrandFolder(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "取消保留分类历史")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	store := projectCopyStore(db)
	service := workspaceapp.NewProjectCopyService(store, time.Now)
	job, err := service.Create(ctx, actor, copyInput(source))
	if err != nil {
		t.Fatal(err)
	}
	requested, err := service.Change(ctx, actor, job.ID, "cancel", job.Revision, uuid.New(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	claimed, err := store.Claim(ctx, actor, job.ID, worker, false, time.Now())
	if err != nil || claimed.Status != "cancel_requested" {
		t.Fatalf("claim cleanup %+v %v", claimed, err)
	}
	cancelled, err := store.FinishCancelled(ctx, actor, job.ID, worker)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("real cleanup %+v %v", cancelled, err)
	}
	var target struct {
		IsDelete bool
		Revision int64
	}
	if err = db.Raw(`SELECT is_delete,revision FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&target).Error; err != nil || !target.IsDelete {
		t.Fatalf("cancelled target not recycled %+v %v", target, err)
	}
	var p domain.FolderPlacement
	if err = db.Raw(`SELECT org_id,actor_id,project_id,folder_id,revision FROM workspace.project_folder_placement WHERE actor_id=? AND project_id=?`, actor.ID, job.TargetProjectID).Scan(&p).Error; err != nil || p.FolderID == nil || *p.FolderID != f.ID || p.Revision != 1 {
		t.Fatalf("cleanup changed frozen classification %+v %v", p, err)
	}
	list, err := folders.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || list.Items[0].ProjectCount != 1 {
		t.Fatalf("deleted copy counted %+v %v", list, err)
	}
	var unchanged int64
	if err = db.Raw(`SELECT revision FROM workspace.project_folder WHERE id=?`, f.ID).Scan(&unchanged).Error; err != nil || unchanged != f.Revision+1 {
		t.Fatalf("worker took/revised directory %+v %v", unchanged, err)
	}
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = unchanged
	if _, err = folders.Change(ctx, actor, recycle); err != nil {
		t.Fatalf("cancelled target stranded directory %v", err)
	}
	_ = requested
}

func TestProjectCopyPlacementPGExplicitCASRootAndOuterRollback(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "重名分类")
	createTestFolder(ctx, t, folders, actor, "重名分类")
	f = *moveTestProject(ctx, t, folders, actor, source, f, 0).Folder
	service := workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now)
	for _, expect := range []workspaceapp.CopyPlacementExpectation{{ExpectedPlacementRevision: 0, FolderID: &f.ID, ExpectedFolderRevision: f.Revision}, {ExpectedPlacementRevision: 1, FolderID: &f.ID, ExpectedFolderRevision: f.Revision - 1}, {ExpectedPlacementRevision: 1, ExpectedFolderRevision: 0}} {
		in := copyInput(source)
		in.Placement = &expect
		if _, err := service.Create(ctx, actor, in); err == nil {
			t.Fatalf("stale classification accepted %+v", expect)
		}
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project_copy_job WHERE actor_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected CAS left job %d %v", count, err)
	}
	in := copyInput(source)
	in.Placement = &workspaceapp.CopyPlacementExpectation{ExpectedPlacementRevision: 1, FolderID: &f.ID, ExpectedFolderRevision: f.Revision}
	rollback := errors.New("outer rollback")
	var target uuid.UUID
	err := db.Transaction(func(tx *gorm.DB) error {
		job, err := workspaceapp.NewProjectCopyService(projectCopyStore(tx), time.Now).Create(ctx, actor, in)
		if err != nil {
			return err
		}
		target = job.TargetProjectID
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err = db.Raw(`SELECT count(*) FROM workspace.project_folder_placement WHERE project_id=?`, target).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("outer rollback left placement %d %v", count, err)
	}
	var rev int64
	if err = db.Raw(`SELECT revision FROM workspace.project_folder WHERE id=?`, f.ID).Scan(&rev).Error; err != nil || rev != f.Revision {
		t.Fatalf("outer rollback changed membership CAS %d %v", rev, err)
	}
	if err = db.Exec(`UPDATE workspace.project_copy_job SET workspace_snapshot='{}' WHERE actor_id=?`, actor.ID).Error; err == nil {
		t.Fatal("runtime can rewrite frozen Copy content")
	}
	move := folderCommand("move")
	move.ProjectID = source
	move.ExpectedProjectRevision = 1
	move.ExpectedPlacementRevision = 1
	if _, err = folders.Change(ctx, actor, move); err != nil {
		t.Fatal(err)
	}
	root := copyInput(source)
	root.Placement = &workspaceapp.CopyPlacementExpectation{ExpectedPlacementRevision: 2, ExpectedFolderRevision: 0}
	job, err := service.Create(ctx, actor, root)
	if err != nil {
		t.Fatal(err)
	}
	var p domain.FolderPlacement
	if err = db.Raw(`SELECT org_id,actor_id,project_id,folder_id,revision FROM workspace.project_folder_placement WHERE project_id=?`, job.TargetProjectID).Scan(&p).Error; err != nil || p.FolderID != nil || p.Revision != 1 {
		t.Fatalf("explicit root %+v %v", p, err)
	}
}
