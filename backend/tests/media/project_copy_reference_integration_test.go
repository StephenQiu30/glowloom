package media_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestProjectCopyTypedHistoricalReferencesIncludeUncatalogedImageAndAudio(t *testing.T) {
	for _, kind := range []domain.Kind{domain.KindImage, domain.KindAudio} {
		t.Run(string(kind), func(t *testing.T) {
			db := libraryRuntimeDB(t)
			owner := libraryOwnerDB(t)
			objects := glbTestObjects(t)
			actor, project := mediaStoreProject(t, db)
			name, body := "legacy-reference.png", uploadPNG(t)
			if kind == domain.KindAudio {
				name = "legacy-reference.wav"
				file := filepath.Join(t.TempDir(), name)
				if err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=880:duration=1", "-c:a", "pcm_s16le", file).Run(); err != nil {
					t.Fatal(err)
				}
				var err error
				body, err = os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
			}
			upload := documentUpload(t, db, objects, actor, project, name, body)
			var fact mediaapp.ReferenceFact
			if err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				fact, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, upload.Asset.ID, kind)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			base := pgmedia.NewStore(db)
			asset, err := base.FindAsset(t.Context(), actor, project, fact.AssetID)
			if err != nil {
				t.Fatal(err)
			}
			rends, err := base.FindRenditions(t.Context(), actor, project, fact.AssetID)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rends {
				t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
			}
			var catalog int64
			if err := db.Raw(`SELECT count(*) FROM media.library_item WHERE asset_id=?`, asset.ID).Scan(&catalog).Error; err != nil || catalog != 0 {
				t.Fatal("fixture already had owning catalog declaration", catalog, err)
			}
			deleted, err := asset.Delete(time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=?`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, asset.ID).Error; err != nil {
				t.Fatal(err)
			}
			binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
			if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'无目录历史媒体完整副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				_, err := pgmedia.NewProjectCopyStore(tx).FreezeWithReferences(t.Context(), actor, binding, time.Now(), []uuid.UUID{fact.AssetID})
				return err
			}); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
				t.Fatal("document-only caller was widened", err)
			}
			bad := fact
			bad.SHA256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
			if err := db.Transaction(func(tx *gorm.DB) error {
				_, err := pgmedia.NewProjectCopyStore(tx).FreezeWithReferenceFacts(t.Context(), actor, binding, time.Now(), nil, []mediaapp.ReferenceFact{bad})
				return err
			}); !errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable) {
				t.Fatal("mismatched immutable original accepted", err)
			}
			var count int64
			if err := db.Raw(`SELECT count(*) FROM media.project_copy_snapshot WHERE job_id=?`, binding.JobID).Scan(&count).Error; err != nil || count != 0 {
				t.Fatal("rejected typed source left a partial snapshot", count, err)
			}
			var snapshot mediaapp.ProjectCopySnapshot
			var mappings []mediaapp.ProjectCopyReferenceMapping
			if err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				repo := pgmedia.NewProjectCopyStore(tx)
				snapshot, err = repo.FreezeWithReferenceFacts(t.Context(), actor, binding, time.Now(), nil, []mediaapp.ReferenceFact{fact})
				if err != nil {
					return err
				}
				mappings, err = mediaapp.NewProjectCopyReferenceQuery(repo, nil).FreezeReferences(t.Context(), actor, binding, snapshot, []mediaapp.ReferenceFact{fact})
				if err != nil {
					return err
				}
				return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'无目录历史媒体完整副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
			}); err != nil || snapshot.Assets != 1 || len(mappings) != 1 || mappings[0].Source.Revision != fact.Revision {
				t.Fatal("trusted uncataloged historical source omitted", snapshot, mappings, err)
			}
			repo := pgmedia.NewProjectCopyStore(db)
			if err := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir()).Transfer(t.Context(), actor, binding, snapshot); err != nil {
				t.Fatal(err)
			}
			intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, object := range intents {
				t.Cleanup(func() { _ = objects.Remove(context.Background(), object.TargetObjectKey) })
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := mediaapp.NewProjectCopyReferenceQuery(repo, objects).VerifyTransferred(t.Context(), actor, binding, snapshot, mappings); err != nil {
				t.Fatal("actual original/rendition retained proof", err)
			}
			if _, err := base.FindAsset(t.Context(), actor, project, fact.AssetID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("special copy broadened ordinary original read", err)
			}
		})
	}
}

func TestProjectCopyFrozenReferencesKeepHistoricalRevisionAndProveActualTransferredBytes(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	upload := documentUpload(t, db, objects, actor, project, "copy-reference.png", uploadPNG(t))
	var fact mediaapp.ReferenceFact
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		fact, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, upload.Asset.ID, domain.KindImage)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base := pgmedia.NewStore(db)
	asset, err := base.FindAsset(t.Context(), actor, project, fact.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	rends, err := base.FindRenditions(t.Context(), actor, project, fact.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	if _, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "remove_items", Items: []mediaapp.LibraryItemRevision{{ID: asset.ID}}}); err != nil {
		t.Fatal(err)
	}
	deleted, err := asset.Delete(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=?`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'历史引用完整副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot mediaapp.ProjectCopySnapshot
	var mapping []mediaapp.ProjectCopyReferenceMapping
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		repo := pgmedia.NewProjectCopyStore(tx)
		snapshot, err = repo.Freeze(t.Context(), actor, binding, time.Now())
		if err != nil {
			return err
		}
		// No object getter is supplied: admission must remain SQL-only.
		mapping, err = mediaapp.NewProjectCopyReferenceQuery(repo, nil).FreezeReferences(t.Context(), actor, binding, snapshot, []mediaapp.ReferenceFact{fact})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'历史引用完整副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal("SQL-only historical mapping", err)
	}
	if len(mapping) != 1 || !reflect.DeepEqual(mapping[0].Source, fact) || mapping[0].Target.AssetID != snapshot.AssetMapping[fact.AssetID] || mapping[0].Target.Revision != 1 || mapping[0].Target.RenditionID == nil || *mapping[0].Target.RenditionID == *fact.RenditionID || *mapping[0].Target.RenditionSHA256 != *fact.RenditionSHA256 {
		t.Fatal("owner mapping changed source history or guessed target evidence", mapping)
	}
	repo := pgmedia.NewProjectCopyStore(db)
	query := mediaapp.NewProjectCopyReferenceQuery(repo, objects)
	if err := query.VerifyTransferred(t.Context(), actor, binding, snapshot, mapping); err == nil {
		t.Fatal("unregistered/untransferred references passed verification")
	}
	if err := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir()).Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range intents {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), object.TargetObjectKey) })
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := query.VerifyTransferred(t.Context(), actor, binding, snapshot, mapping); err != nil {
		t.Fatal("actual retained bytes and independent target rendition", err)
	}
	changed := append([]mediaapp.ProjectCopyReferenceMapping(nil), mapping...)
	digest := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	changed[0].Source.RenditionSHA256 = &digest
	changed[0].Target.RenditionSHA256 = &digest
	if err := query.VerifyTransferred(t.Context(), actor, binding, snapshot, changed); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("unproven expected rendition digest became transferred evidence", err)
	}
	if err := objects.Remove(t.Context(), intents[0].TargetObjectKey); err != nil {
		t.Fatal(err)
	}
	if err := query.VerifyTransferred(t.Context(), actor, binding, snapshot, mapping); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("physically missing independent target accepted", err)
	}
	if intents[0].RenditionKind != "" {
		t.Fatal("expected original intent first")
	}
	if err := objects.PutIfAbsent(t.Context(), intents[0].TargetObjectKey, bytes.NewReader(uploadPNG(t)), fact.ByteSize, "image/png", fact.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := objects.Remove(t.Context(), asset.ObjectKey); err != nil {
		t.Fatal(err)
	}
	if err := query.VerifyTransferred(t.Context(), actor, binding, snapshot, mapping); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("physically missing historical source accepted before publish", err)
	}
}
