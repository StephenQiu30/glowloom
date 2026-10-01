package workspace_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	operationpg "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func folderTestDB(t *testing.T) (context.Context, *gorm.DB, *gorm.DB) {
	t.Helper()
	runtime, owner := os.Getenv("LV_TEST_FOLDER_DB_DSN"), os.Getenv("LV_TEST_FOLDER_OWNER_DSN")
	if runtime == "" || owner == "" {
		t.Skip("set LV_TEST_FOLDER_DB_DSN and LV_TEST_FOLDER_OWNER_DSN to isolated migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	open := func(dsn string) *gorm.DB {
		c, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.DB.WithContext(ctx)
	}
	r, o := open(runtime), open(owner)
	var current string
	if err := r.Raw(`SELECT current_user`).Scan(&current).Error; err != nil || current != "lanverse_app" {
		t.Fatalf("test must use runtime role: %s %v", current, err)
	}
	return ctx, r, o
}
func folderWork(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
	return owningProjectWork{operationpg.NewStore(tx), mediapg.NewStore(tx), toolpg.NewStore(tx, nil, nil), toolpg.NewTranscriptionStore(tx, nil, nil), toolpg.NewDepthStore(tx, nil, nil), workspacepg.NewProjectCopyStore(tx, nil, nil)}
}
func folderStore(db *gorm.DB) *workspacepg.FolderStore {
	return workspacepg.NewFolderStore(db, folderWork, func(tx *gorm.DB) workspaceapp.FolderMediaReader {
		return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil)
	})
}
func folderCommand(action string) workspaceapp.FolderChangeInput {
	return workspaceapp.FolderChangeInput{Action: action, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
}
func createTestFolder(ctx context.Context, t *testing.T, service *workspaceapp.ProjectFolders, actor identityapp.Principal, name string) domain.ProjectFolder {
	t.Helper()
	in := folderCommand("create")
	in.Name = &name
	out, err := service.Change(ctx, actor, in)
	if err != nil || out.Folder == nil {
		t.Fatalf("create folder %v", err)
	}
	return *out.Folder
}
func moveTestProject(ctx context.Context, t *testing.T, service *workspaceapp.ProjectFolders, actor identityapp.Principal, project uuid.UUID, folder domain.ProjectFolder, placement int64) workspaceapp.FolderChangeResult {
	t.Helper()
	in := folderCommand("move")
	in.ProjectID = project
	in.ExpectedProjectRevision = 1
	in.ExpectedPlacementRevision = placement
	in.FolderID = folder.ID
	in.ExpectedRevision = folder.Revision
	out, err := service.Change(ctx, actor, in)
	if err != nil {
		t.Fatalf("move folder %v", err)
	}
	return out
}

func TestProjectFolderPGPlacementPaginationAndDurableIdentity(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	other := insertWorkspaceActor(ctx, t, owner, actor.OrgID.String())
	ids := []uuid.UUID{uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil)), uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil)), uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))}
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "可重名目录")
	same := createTestFolder(ctx, t, service, actor, "可重名目录")
	if f.ID == same.ID {
		t.Fatal("equal names merged identity")
	}
	out := moveTestProject(ctx, t, service, actor, ids[0], f, 0)
	f = *out.Folder
	out = moveTestProject(ctx, t, service, actor, ids[1], f, 0)
	f = *out.Folder
	query := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db))
	page, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &f.ID, Limit: 1})
	if err != nil || len(page.Projects) != 1 || page.Next == nil || page.Projects[0].FolderID == nil || page.Projects[0].PlacementRevision != 1 {
		t.Fatalf("folder page %+v %v", page, err)
	}
	next, err := query.Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &f.ID, Limit: 1, After: page.Next})
	if err != nil || len(next.Projects) != 1 || next.Projects[0].ID == page.Projects[0].ID || next.Next != nil {
		t.Fatalf("next page %+v %v", next, err)
	}
	root := uuid.Nil
	page, err = query.Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &root, Limit: 50})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].ID != ids[2] {
		t.Fatalf("root page %+v %v", page, err)
	}
	page, err = query.Execute(ctx, other, workspaceapp.ListProjectsInput{FolderID: &root, Limit: 50})
	if err != nil || len(page.Projects) != 3 {
		t.Fatalf("personal placements leaked %+v %v", page, err)
	}
	if _, err = query.Execute(ctx, other, workspaceapp.ListProjectsInput{FolderID: &f.ID, Limit: 50}); !errors.Is(err, domain.ErrProjectFolderNotFound) {
		t.Fatalf("other directory %v", err)
	}
	list, err := service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || len(list.Items) != 2 {
		t.Fatalf("directory list %+v %v", list, err)
	}
	for _, item := range list.Items {
		if item.Folder.ID == f.ID && item.ProjectCount != 2 {
			t.Fatalf("membership count %d", item.ProjectCount)
		}
	}
	rename := folderCommand("patch")
	rename.FolderID = f.ID
	rename.ExpectedRevision = f.Revision
	name := "真实更名"
	rename.Name = &name
	saved, err := service.Change(ctx, actor, rename)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := workspaceapp.NewProjectFolders(folderStore(db), time.Now).Change(ctx, actor, rename)
	if err != nil || !replay.Folder.UpdateTime.Equal(saved.Folder.UpdateTime) {
		t.Fatalf("persistent replay %+v %v", replay, err)
	}
	changed := rename
	different := "同键异输入"
	changed.Name = &different
	if _, err = service.Change(ctx, actor, changed); !errors.Is(err, workspaceapp.ErrIdempotencyConflict) {
		t.Fatalf("changed receipt %v", err)
	}
	var projectRev int64
	if err = db.Raw(`SELECT revision FROM workspace.project WHERE id=?`, ids[0]).Scan(&projectRev).Error; err != nil || projectRev != 1 {
		t.Fatalf("classification changed project content revision %d %v", projectRev, err)
	}
	if err = db.Exec(`UPDATE workspace.project_folder_command SET action='recycle' WHERE actor_id=?`, actor.ID).Error; err == nil {
		t.Fatal("runtime rewrote permanent receipt")
	}
	if err = owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Change(ctx, actor, rename); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked replay %v", err)
	}
}

func TestProjectFolderPGWholeRecycleAtomicGuardsAndRestore(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "全成员回收")
	ids := []uuid.UUID{uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil)), uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))}
	// Choose the later lock-order member so earlier tentative lifecycle writes must roll back.
	if ids[0].String() > ids[1].String() {
		ids[0], ids[1] = ids[1], ids[0]
	}
	for _, id := range ids {
		out := moveTestProject(ctx, t, service, actor, id, f, 0)
		f = *out.Folder
	}
	operation, asset, export, transcription, depth, copyID, target := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	fixture := func(query string, args ...any) { t.Helper(); lifecycleFixtureSQL(t, owner, query, args...) }
	fixture(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,quote_micros,quote_expires_at)VALUES(?,?,'free','image.generate','text_to_image','folder-guard','upload','completed',0,clock_timestamp()+interval '1 hour')`, operation, ids[1])
	fixture(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size)VALUES(?,?,'image','upload','ready',?,'image/png',12)`, asset, ids[1], "folder-tests/"+asset.String())
	fixture(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,'{}','failed','failed',0,1,1,clock_timestamp(),clock_timestamp())`, export, ids[1], actor.OrgID, actor.ID, uuid.New(), uuid.New())
	fixture(`INSERT INTO mediatool.transcription_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,language,frozen,status,stage,progress,attempt,revision,inference_state,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,'zh','{}','failed','failed',0,1,1,'terminal',clock_timestamp(),clock_timestamp())`, transcription, ids[1], actor.OrgID, actor.ID, uuid.New(), uuid.New())
	fixture(`INSERT INTO mediatool.depth_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,source_asset_id,source_asset_revision,source_sha256,profile_id,frozen,frozen_sha256,status,stage,attempt,revision,process_state,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,?,1,repeat('a',64),'vda-small-relative-v1','{}',repeat('b',64),'failed','failed',1,1,'ended',clock_timestamp(),clock_timestamp())`, depth, ids[1], actor.OrgID, actor.ID, uuid.New(), uuid.New(), asset)
	fixture(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,idem_key,request_sha256,admission_response,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot)VALUES(?,?,?,?,repeat('c',64),'{}',?,1,?,'copy guard','cancelled','cleanup','{}','{}')`, copyID, actor.OrgID, actor.ID, uuid.New(), ids[1], target)
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = f.Revision
	for _, group := range []struct {
		table         string
		id            uuid.UUID
		blocked, idle string
	}{
		{"operation.operation", operation, "unknown", "completed"}, {"media.media_asset", asset, "processing", "ready"}, {"mediatool.export_job", export, "cancel_requested", "failed"}, {"mediatool.transcription_job", transcription, "running", "failed"}, {"mediatool.depth_job", depth, "review_required", "failed"}, {"workspace.project_copy_job", copyID, "failed", "cancelled"},
	} {
		t.Run(group.table, func(t *testing.T) {
			fixture("UPDATE "+group.table+" SET status=? WHERE id=?", group.blocked, group.id)
			if _, err := service.Change(ctx, actor, recycle); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
				t.Fatalf("real owning guard ignored: %v", err)
			}
			var counts struct{ Deleted, Receipts, Placements, Changes int64 }
			if err := db.Raw(`SELECT(SELECT count(*) FROM workspace.project WHERE id IN (?,?) AND is_delete)AS deleted,(SELECT count(*) FROM workspace.project_folder_command WHERE actor_id=? AND idem_key=?)AS receipts,(SELECT count(*) FROM workspace.project_folder_placement WHERE actor_id=? AND folder_id=?)AS placements,(SELECT count(*) FROM infra.outbox WHERE topic='lanverse.workspace.project_changed.v1' AND partition_key IN (?,?))AS changes`, ids[0], ids[1], actor.ID, recycle.IdempotencyKey, actor.ID, f.ID, ids[0].String(), ids[1].String()).Scan(&counts).Error; err != nil {
				t.Fatal(err)
			}
			if counts.Deleted != 0 || counts.Receipts != 0 || counts.Placements != 2 || counts.Changes != 0 {
				t.Fatalf("partial rollback %+v", counts)
			}
			fixture("UPDATE "+group.table+" SET status=? WHERE id=?", group.idle, group.id)
		})
	}
	// Unknown native process facts remain a fence even when a job reports failure.
	fixture(`UPDATE mediatool.depth_job SET needs_reconciliation=true,execution_unconfirmed=true,active_worker=?,process_state='unknown' WHERE id=?`, uuid.New(), depth)
	if _, err := service.Change(ctx, actor, recycle); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("unknown native process admitted: %v", err)
	}
	fixture(`UPDATE mediatool.depth_job SET needs_reconciliation=false,execution_unconfirmed=false,active_worker=NULL,process_state='ended' WHERE id=?`, depth)
	out, err := service.Change(ctx, actor, recycle)
	if err != nil || out.Folder == nil || !out.Folder.IsDelete || len(out.RecycledProjectIDs) != 2 {
		t.Fatalf("whole recycle %+v %v", out, err)
	}
	replay, err := service.Change(ctx, actor, recycle)
	if err != nil || !replay.Folder.UpdateTime.Equal(out.Folder.UpdateTime) {
		t.Fatalf("deleted directory replay %+v %v", replay, err)
	}
	lifecycle := workspaceapp.NewProjectLifecycle(workspacepg.NewStoreWithProjectWorkGuard(db, folderWork), time.Now)
	for _, id := range ids {
		restored, err := lifecycle.Change(ctx, actor, projectChange(id, 2, "restore"))
		if err != nil || restored.Project.IsDelete {
			t.Fatalf("restore %+v %v", restored, err)
		}
	}
	root := uuid.Nil
	page, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &root, Limit: 50})
	if err != nil || len(page.Projects) != 3 {
		t.Fatalf("restored root %+v %v", page, err)
	}
	for _, p := range page.Projects {
		if p.ID != target && (p.FolderID != nil || p.PlacementRevision != 2) {
			t.Fatalf("restored classification %+v", p)
		}
	}
	if _, err := service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50}); err != nil {
		t.Fatal(err)
	}
}

func TestProjectFolderPGFormalCoverAndIndividualRestore(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	asset := uuid.New()
	lifecycleFixtureSQL(t, owner, `INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,moderation_status,sha256,width,height)VALUES(?,?,'image','upload','ready',?,'image/png',12,'passed',repeat('a',64),32,32)`, asset, project, "projects/"+project.String()+"/image/2026/10/"+asset.String()+".png")
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	in := folderCommand("create")
	name := "正式封面"
	in.Name = &name
	in.SetCover = true
	in.Cover = &domain.FolderCover{ProjectID: project, AssetID: asset}
	out, err := service.Change(ctx, actor, in)
	if err != nil || out.Folder.Cover == nil {
		t.Fatalf("ready image cover %+v %v", out, err)
	}
	f := *moveTestProject(ctx, t, service, actor, project, *out.Folder, 0).Folder
	lifecycle := workspaceapp.NewProjectLifecycle(workspacepg.NewStoreWithProjectWorkGuard(db, folderWork), time.Now)
	if _, err = lifecycle.Change(ctx, actor, projectChange(project, 1, "delete")); err != nil {
		t.Fatal(err)
	}
	list, err := service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || len(list.Items) != 1 || !list.Items[0].CoverUnavailable || list.Items[0].Folder.Cover != nil || list.Items[0].ProjectCount != 0 {
		t.Fatalf("revoked project cover %+v %v", list, err)
	}
	if _, err = lifecycle.Change(ctx, actor, projectChange(project, 2, "restore")); err != nil {
		t.Fatal(err)
	}
	list, err = service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || list.Items[0].CoverUnavailable || list.Items[0].Folder.Cover == nil || list.Items[0].ProjectCount != 1 {
		t.Fatalf("restored original folder %+v %v", list, err)
	}
	page, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &f.ID, Limit: 50})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].PlacementRevision != 1 {
		t.Fatalf("individual lifecycle changed placement %+v %v", page, err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, asset)
	list, err = service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || !list.Items[0].CoverUnavailable || list.Items[0].Folder.Cover != nil {
		t.Fatalf("unreviewed image retained cover %+v %v", list, err)
	}
	patch := folderCommand("patch")
	patch.FolderID = f.ID
	patch.ExpectedRevision = f.Revision
	patch.SetCover = true
	patch.Cover = in.Cover
	if _, err = service.Change(ctx, actor, patch); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("new unreviewed cover accepted %v", err)
	}
	patch.Cover = nil
	cleared, err := service.Change(ctx, actor, patch)
	if err != nil || cleared.Folder.Cover != nil {
		t.Fatalf("explicit removal %+v %v", cleared, err)
	}
	foreign := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	foreignProject := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, foreign.OrgID.String(), "16:9", "realistic", nil))
	patch = folderCommand("patch")
	patch.FolderID = f.ID
	patch.ExpectedRevision = cleared.Folder.Revision
	patch.SetCover = true
	patch.Cover = &domain.FolderCover{ProjectID: foreignProject, AssetID: asset}
	if _, err = service.Change(ctx, actor, patch); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("foreign cover disclosed %v", err)
	}
	var count int64
	if err = db.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND NOT is_delete`, asset).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("directory removed original asset", err)
	}
}

func TestProjectFolderPGConcurrentPermanentCommandsAndCurrentOrganization(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	in := folderCommand("create")
	name := "并发持久请求"
	in.Name = &name
	const count = 6
	type result struct {
		out workspaceapp.FolderChangeResult
		err error
	}
	done := make(chan result, count)
	for range count {
		go func() { out, err := service.Change(ctx, actor, in); done <- result{out, err} }()
	}
	completed := make([]result, 0, count)
	for range count {
		completed = append(completed, <-done)
	}
	var original *domain.ProjectFolder
	for _, r := range completed {
		if r.err != nil || r.out.Folder == nil {
			t.Fatalf("concurrent receipt %v", r.err)
		}
		if original == nil {
			original = r.out.Folder
		} else if r.out.Folder.ID != original.ID || !r.out.Folder.UpdateTime.Equal(original.UpdateTime) {
			t.Fatal("same key changed accepted identity")
		}
	}
	var totals struct{ Folders, Receipts, Events int64 }
	if err := db.Raw(`SELECT(SELECT count(*) FROM workspace.project_folder WHERE actor_id=?)AS folders,(SELECT count(*) FROM workspace.project_folder_command WHERE actor_id=?)AS receipts,(SELECT count(*) FROM infra.outbox WHERE payload->'actor'->>'id'=?)AS events`, actor.ID, actor.ID, actor.ID.String()).Scan(&totals).Error; err != nil || totals.Folders != 1 || totals.Receipts != 1 || totals.Events != 1 {
		t.Fatalf("concurrent duplicate commit %+v %v", totals, err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.organization SET status='disabled' WHERE id=?`, actor.OrgID)
	if _, err := service.Change(ctx, actor, in); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("disabled organization replay %v", err)
	}
	if _, err := service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("disabled org list %v", err)
	}
}

func TestProjectFolderPGArchivedMemberPreservesLifecycleAndCopyingFence(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "归档成员")
	f = *moveTestProject(ctx, t, service, actor, project, f, 0).Folder
	lifecycle := workspaceapp.NewProjectLifecycle(workspacepg.NewStoreWithProjectWorkGuard(db, folderWork), time.Now)
	if _, err := lifecycle.Change(ctx, actor, projectChange(project, 1, "archive")); err != nil {
		t.Fatal(err)
	}
	move := folderCommand("move")
	move.ProjectID = project
	move.ExpectedProjectRevision = 2
	move.ExpectedPlacementRevision = 1
	if _, err := service.Change(ctx, actor, move); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("archived navigation mutation %v", err)
	}
	recycle := folderCommand("recycle")
	recycle.FolderID = f.ID
	recycle.ExpectedRevision = f.Revision
	copying := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET status='copying' WHERE id=?`, copying)
	// A copy target is admitted by Copy's separate transaction port, never by public Move.
	lifecycleFixtureSQL(t, owner, `INSERT INTO workspace.project_folder_placement(org_id,actor_id,project_id,folder_id)VALUES(?,?,?,?)`, actor.OrgID, actor.ID, copying, f.ID)
	list, err := service.List(ctx, actor, workspaceapp.FolderListInput{Limit: 50})
	if err != nil || list.Items[0].ProjectCount != 1 {
		t.Fatalf("copying published in directory count %+v %v", list, err)
	}
	if _, err = service.Change(ctx, actor, recycle); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("unpublished copy target admitted %v", err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET status='active' WHERE id=?`, copying)
	if _, err = service.Change(ctx, actor, recycle); err != nil {
		t.Fatal(err)
	}
	restored, err := lifecycle.Change(ctx, actor, projectChange(project, 3, "restore"))
	if err != nil || restored.Project.Status != "archived" || restored.Project.ArchivedAt == nil || restored.Project.PurgeAfter != nil {
		t.Fatalf("archived restoration %+v %v", restored, err)
	}
	root := uuid.Nil
	page, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &root, Status: "archived", Limit: 50})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].ID != project {
		t.Fatalf("restored archived root %+v %v", page, err)
	}
}
