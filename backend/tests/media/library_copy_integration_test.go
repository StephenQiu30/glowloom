package media_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestProjectCopyLibraryRetainedImagePreservesTrashAndIndependentBytes(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	original := documentUpload(t, db, objects, actor, project, "retained.png", uploadPNG(t))
	store := pgmedia.NewStore(db)
	source, err := store.FindAsset(t.Context(), actor, project, original.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, project, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "update_item", Key: uuid.New(), ItemID: &source.ID, Metadata: &mediaapp.LibraryMetadata{Title: "回收的原图", Category: "material", Tags: []string{"保留"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "recycle_items", Key: uuid.New(), ExpectedRevision: 1, Items: []mediaapp.LibraryItemRevision{{ID: source.ID, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	deleted, err := source.Delete(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=? AND project_id=?`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, source.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindAsset(t.Context(), actor, project, source.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("ordinary read exposed retained image", err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'完整回收库副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).FreezeWithReferences(t.Context(), actor, binding, time.Now(), []uuid.UUID{source.ID})
		return err
	}); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
		t.Fatal("external Script history input accepted a non-document", err)
	}
	var snapshot mediaapp.ProjectCopySnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT id FROM workspace.project WHERE id IN ? ORDER BY id FOR UPDATE`, []uuid.UUID{project, binding.TargetProjectID}).Scan(new([]uuid.UUID)).Error; err != nil {
			return err
		}
		var err error
		snapshot, err = pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'完整回收库副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal("owning library must include retained original", err)
	}
	if snapshot.Assets != 1 || snapshot.Renditions != len(rends) {
		t.Fatal("incomplete retained original and renditions", snapshot)
	}
	repo := pgmedia.NewProjectCopyStore(db)
	transfer := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir())
	if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, item := range intents {
			_ = objects.Remove(context.Background(), item.TargetObjectKey)
		}
	})
	rollback := errors.New("synthetic coordinator rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal("owning transaction could not roll back its full catalog registration", err)
	}
	var registered int64
	if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=?`, binding.TargetProjectID).Scan(&registered).Error; err != nil || registered != 0 {
		t.Fatal("outer rollback published target media", registered, err)
	}
	if err := db.Raw(`SELECT count(*) FROM media.project_copy_receipt WHERE snapshot_id=?`, snapshot.ID).Scan(&registered).Error; err != nil || registered != 0 {
		t.Fatal("outer rollback left a permanent media registration receipt", registered, err)
	}
	privateScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	privateLibraryID, _ := privateScope.Identity(actor.OrgID, actor.ID)
	if err := db.Raw(`SELECT count(*) FROM media.library_item WHERE library_id=?`, privateLibraryID).Scan(&registered).Error; err != nil || registered != 0 {
		t.Fatal("outer rollback published catalog independently", registered, err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, binding.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	targetID := snapshot.AssetMapping[source.ID]
	targetScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	item, err := library.LibraryDetail(t.Context(), actor, targetScope, targetID)
	if err != nil || item.State != "trashed" || item.Title != "回收的原图" || item.TrashedAt == nil {
		t.Fatal("copy lost catalog state", item, err)
	}
	page, err := library.ListLibrary(t.Context(), actor, targetScope, libraryQuery())
	if err != nil || page.Total != 0 {
		t.Fatal("trash became ordinary visible catalog content", page, err)
	}
	target, err := store.FindAsset(t.Context(), actor, binding.TargetProjectID, targetID)
	if err != nil || target.IsDelete || target.SHA256 == nil || *target.SHA256 != *source.SHA256 || target.ObjectKey == source.ObjectKey {
		t.Fatal("independent retained target facts", target, err)
	}
	if err := objects.Remove(t.Context(), source.ObjectKey); err != nil {
		t.Fatal(err)
	}
	file, err := objects.Get(t.Context(), target.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(file)
	if err != nil || string(body) != string(uploadPNG(t)) {
		t.Fatal("target lost actual bytes after exact fixture source removal", err)
	}
	var retained bool
	if err := db.Raw(`SELECT is_delete FROM media.media_asset WHERE id=?`, source.ID).Scan(&retained).Error; err != nil || !retained {
		t.Fatal("copy changed source deletion", retained, err)
	}
}

func TestProjectCopyLibraryUnavailableOriginalNeverPublishesPartialMetadata(t *testing.T) {
	for _, failure := range []string{"unapproved", "missing_original", "missing_rendition"} {
		t.Run(failure, func(t *testing.T) {
			db := libraryRuntimeDB(t)
			objects := glbTestObjects(t)
			actor, project := mediaStoreProject(t, db)
			original := documentUpload(t, db, objects, actor, project, "required.png", uploadPNG(t))
			store := pgmedia.NewStore(db)
			source, err := store.FindAsset(t.Context(), actor, project, original.Asset.ID)
			if err != nil {
				t.Fatal(err)
			}
			rends, err := store.FindRenditions(t.Context(), actor, project, source.ID)
			if err != nil || len(rends) != 2 {
				t.Fatal("real source derivatives", err)
			}
			for _, r := range rends {
				t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
			}
			library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
			scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
			if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "update_item", Key: uuid.New(), ItemID: &source.ID, Metadata: &mediaapp.LibraryMetadata{Title: "不得省略", Category: "other", Tags: []string{}}}); err != nil {
				t.Fatal(err)
			}
			if failure == "unapproved" {
				if err := libraryOwnerDB(t).Exec(`UPDATE media.media_asset SET moderation_status='pending' WHERE id=?`, source.ID).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				key := source.ObjectKey
				if failure == "missing_rendition" {
					key = rends[0].ObjectKey
				}
				if err := objects.Remove(t.Context(), key); err != nil {
					t.Fatal(err)
				}
			}
			binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
			if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'不完整不发布','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
				t.Fatal(err)
			}
			var snapshot mediaapp.ProjectCopySnapshot
			freezeErr := db.Transaction(func(tx *gorm.DB) error {
				var err error
				snapshot, err = pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
				if err != nil {
					return err
				}
				return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'不完整不发布','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
			})
			if failure == "unapproved" {
				if !errors.Is(freezeErr, mediaapp.ErrProjectCopyMediaUnavailable) {
					t.Fatal("unapproved source admitted", freezeErr)
				}
				var count int64
				if err := db.Raw(`SELECT count(*) FROM media.project_copy_snapshot WHERE job_id=?`, binding.JobID).Scan(&count).Error; err != nil || count != 0 {
					t.Fatal("failed admission left a partial manifest", count, err)
				}
				return
			}
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			repo := pgmedia.NewProjectCopyStore(db)
			transfer := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir())
			if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err == nil {
				t.Fatal("missing required bytes copied as success")
			}
			intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				for _, intent := range intents {
					_ = objects.Remove(context.Background(), intent.TargetObjectKey)
				}
			})
			if err := db.Transaction(func(tx *gorm.DB) error {
				_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
				return err
			}); err == nil {
				t.Fatal("incomplete bytes published catalog")
			}
			var assets, items, receipts int64
			if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=?`, binding.TargetProjectID).Scan(&assets).Error; err != nil {
				t.Fatal(err)
			}
			targetScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
			libraryID, _ := targetScope.Identity(actor.OrgID, actor.ID)
			if err := db.Raw(`SELECT count(*) FROM media.library_item WHERE library_id=?`, libraryID).Scan(&items).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Raw(`SELECT count(*) FROM media.project_copy_receipt WHERE snapshot_id=?`, snapshot.ID).Scan(&receipts).Error; err != nil {
				t.Fatal(err)
			}
			if assets != 0 || items != 0 || receipts != 0 {
				t.Fatal("partial target escaped unpublished object intents", assets, items, receipts)
			}
		})
	}
}

func TestProjectCopyLibraryPreservesFrozenFoldersTextAndIndependentBinaryMetadata(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	original := documentUpload(t, db, objects, actor, project, "source.png", uploadPNG(t))
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	folder, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "create_folder", Key: uuid.New(), Folder: &mediaapp.LibraryFolderInput{Name: "分镜素材", Style: "cinema", Theme: "ember"}})
	if err != nil {
		t.Fatal(err)
	}
	metadata := &mediaapp.LibraryMetadata{FolderID: &folder.Folder.ID, Title: "冻结的展示标题", Category: "prop", Tags: []string{"道具", "原件"}, SourceLabel: "用户来源", Note: "独立元数据"}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "update_item", Key: uuid.New(), ExpectedRevision: 1, ItemID: &original.Asset.ID, Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	text := "第一集对白\n第二行原文"
	plain, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "create_text", Key: uuid.New(), ExpectedRevision: 2, Metadata: &mediaapp.LibraryMetadata{PlainText: &text, FolderID: &folder.Folder.ID, Title: "对白原文", Category: "other", Tags: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'完整库复制','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot mediaapp.ProjectCopySnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT id FROM workspace.project WHERE id IN ? ORDER BY id FOR UPDATE`, []uuid.UUID{project, binding.TargetProjectID}).Scan(new([]uuid.UUID)).Error; err != nil {
			return err
		}
		var err error
		snapshot, err = pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,4,?,'完整库复制','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal(err)
	}
	// Later source editing cannot replace the accepted manifest's metadata.
	metadata.Title = "源后续修改"
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Action: "update_item", Key: uuid.New(), ExpectedRevision: 3, ItemID: &original.Asset.ID, ExpectedItemRevision: 1, Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	repo := pgmedia.NewProjectCopyStore(db)
	transfer := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir())
	if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, item := range intents {
			_ = objects.Remove(ctx, item.TargetObjectKey)
		}
	})
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	targetScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	if err := db.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, binding.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	page, err := library.ListLibrary(t.Context(), actor, targetScope, libraryQuery())
	if err != nil || page.Total != 2 || len(page.Folders) != 1 || page.Folders[0].ID == folder.Folder.ID || page.Folders[0].Style != "cinema" || page.Folders[0].Theme != "ember" {
		t.Fatal("frozen project library lost folders or text", page, err)
	}
	imageID := snapshot.AssetMapping[original.Asset.ID]
	detail, err := library.LibraryDetail(t.Context(), actor, targetScope, imageID)
	if err != nil || detail.Title != "冻结的展示标题" || detail.FolderID == nil || *detail.FolderID != page.Folders[0].ID || detail.AssetID == nil || *detail.AssetID == original.Asset.ID || detail.Note != "独立元数据" {
		t.Fatal("target metadata points to source or later edits", detail, err)
	}
	var targetText uuid.UUID
	for _, item := range page.Items {
		if item.Kind == "text" {
			targetText = item.ID
		}
	}
	body, err := library.LibraryDetail(t.Context(), actor, targetScope, targetText)
	if err != nil || targetText == plain.Items[0].ID || body.PlainText == nil || *body.PlainText != text {
		t.Fatal("real text clone missing", body, err)
	}
	// Publisher's re-read must catch tampered target metadata, not trust counts.
	if err := db.Exec(`UPDATE workspace.project SET status='copying' WHERE id=?`, binding.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Register(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("stable existing receipt replay", err)
	}
	if err := db.Exec(`UPDATE media.library_item SET title='tampered' WHERE id=?`, imageID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Register(t.Context(), actor, binding, snapshot); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("publisher ignored different library metadata", err)
	}
}
