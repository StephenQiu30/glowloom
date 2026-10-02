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
)

func TestProjectCopyRetainedDocumentActualPrivateBytesAndOrdinaryReadFence(t *testing.T) {
	database := mediaStoreDB(t)
	owner := libraryOwnerDB(t)
	var name, role string
	if err := database.Raw(`SELECT current_database(),current_user`).Row().Scan(&name, &role); err != nil || name != "lanverse_library" || role != "lanverse_app" {
		t.Fatal("retained document test requires isolated library DB and runtime role", name, role, err)
	}
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, database)
	body := []byte("历史原件\n角色：保留它。\n")
	upload := documentUpload(t, database, objects, actor, project, "历史.txt", body)
	source, err := pgmedia.NewStore(database).FindAsset(t.Context(), actor, project, upload.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := source.Delete(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=? AND project_id=?`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, source.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	store := pgmedia.NewStore(database)
	if _, err := store.FindAsset(t.Context(), actor, project, source.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("ordinary read exposed deleted historical document", err)
	}
	if _, err := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(database), objects).Freeze(t.Context(), actor, project, []uuid.UUID{source.ID}); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("ordinary script source freeze exposed retired original", err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'保留历史文档副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	var frozen mediaapp.ProjectCopySnapshot
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		frozen, err = pgmedia.NewProjectCopyStore(tx).FreezeWithReferences(t.Context(), actor, binding, time.Now(), []uuid.UUID{source.ID})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'保留历史文档副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal("freeze exact retained original", err)
	}
	if frozen.Assets != 1 || frozen.AssetMapping[source.ID] == uuid.Nil {
		t.Fatal("retained original was silently omitted", frozen)
	}
	copyStore := pgmedia.NewProjectCopyStore(database)
	transfer := mediaapp.NewProjectCopyTransfer(copyStore, copyobjects.NewProjectCopyObjects(objects), t.TempDir())
	if err := transfer.Transfer(t.Context(), actor, binding, frozen); err != nil {
		t.Fatal("actual SHA verified retained original transfer", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, frozen)
		return err
	}); err != nil {
		t.Fatal("register target facts", err)
	}
	// The own media test publishes only its synthetic project fixture; workspace
	// job guards and full publisher behavior have separate owning integration tests.
	if err := database.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, binding.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	targetID := frozen.AssetMapping[source.ID]
	target, err := store.FindAsset(t.Context(), actor, binding.TargetProjectID, targetID)
	if err != nil || target.IsDelete || target.ObjectKey == source.ObjectKey || target.SHA256 == nil || *target.SHA256 != *source.SHA256 {
		t.Fatal("new independent target missing its exact original facts", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := objects.Remove(ctx, target.ObjectKey); err != nil {
			t.Error("remove exact synthetic copied document")
		}
	})
	if err := objects.Remove(t.Context(), source.ObjectKey); err != nil {
		t.Fatal("remove only this synthetic source object")
	}
	docs := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(database), objects)
	facts, err := docs.Freeze(t.Context(), actor, binding.TargetProjectID, []uuid.UUID{targetID})
	if err != nil {
		t.Fatal(err)
	}
	file, err := docs.Open(t.Context(), actor, binding.TargetProjectID, facts[0])
	if err != nil {
		t.Fatal("independent document download after source removal", err)
	}
	defer func() { _ = file.Close() }()
	actual, err := io.ReadAll(file.File)
	if err != nil || string(actual) != string(body) {
		t.Fatal("target lost historical original bytes", err)
	}
	var retained bool
	if err := database.Raw(`SELECT is_delete FROM media.media_asset WHERE id=?`, source.ID).Scan(&retained).Error; err != nil || !retained {
		t.Fatal("copy rewrote source lifecycle", err)
	}
}

func TestProjectCopyRetainedReferencesMissingForeignAndDuplicateFailAtomically(t *testing.T) {
	database := mediaStoreDB(t)
	var name, role string
	if err := database.Raw(`SELECT current_database(),current_user`).Row().Scan(&name, &role); err != nil || name != "lanverse_library" || role != "lanverse_app" {
		t.Fatal("retained reference test requires isolated library DB and runtime role", name, role, err)
	}
	actor, project := mediaStoreProject(t, database)
	foreignActor, foreignProject := mediaStoreProject(t, database)
	foreign := documentUpload(t, database, glbTestObjects(t), foreignActor, foreignProject, "foreign.txt", []byte("unrelated original"))
	for _, refs := range [][]uuid.UUID{{uuid.New()}, {uuid.Nil}, {project, project}, {foreign.Asset.ID}} {
		binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
		if err := database.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'缺原件不得复制','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Transaction(func(tx *gorm.DB) error {
			_, err := pgmedia.NewProjectCopyStore(tx).FreezeWithReferences(t.Context(), actor, binding, time.Now(), refs)
			return err
		}); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
			t.Fatal("invalid retained reference admitted", err)
		}
		var count int64
		if err := database.Raw(`SELECT count(*) FROM media.project_copy_snapshot WHERE job_id=?`, binding.JobID).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("invalid retained set created partial snapshot", count, err)
		}
	}
}
