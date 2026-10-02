package script_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func TestScriptCopyPGActualImportedDocumentHistoryWarningsAndIndependentObjects(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	objects := scriptobjects.NewStorage(storage)
	word := docxFile(t, `<w:tbl><w:tr><w:tc><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>旧表格正文😀</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`)
	document := scriptDocument(t, db, storage, actor, pid, "历史.docx", word)
	txt := []byte{0xff, 0xfe, 0x2d, 0x4e, 0x87, 0x65}
	text := scriptDocument(t, db, storage, actor, pid, "当前.txt", txt)
	imports := app.NewImportService(scriptImportStore(db, storage), time.Now)
	input := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{document, text}, RightsConfirmed: true}
	j, err := imports.Create(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := scriptImportWorker(db, storage, extract.NewExtractor()).Execute(t.Context(), importDelivery(t, owner, actor, input.Key)); err != nil {
		t.Fatal(err)
	}
	j, err = imports.Get(t.Context(), actor, pid, j.ID)
	if err != nil || j.Status != "succeeded" {
		t.Fatal(j, err)
	}
	old, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil || len(old.Sources) != 2 || len(old.Sources[0].Provenance.Warnings) != 1 || old.Sources[0].Provenance.Warnings[0].Code != "table_layout_flattened" {
		t.Fatal("formal extraction facts missing", old, err)
	}
	sources := app.NewSourceService(scriptStore(db), objects, time.Now)
	lineage := old.Sources[0].LineageID
	deleted, err := sources.Write(t.Context(), actor, app.SourceCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), Action: "delete", ExpectedRevision: j.LatestScriptRevision, BaseVersionID: j.LatestVersionID, LineageID: &lineage})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ScriptRevision != 2 {
		t.Fatal(deleted)
	}
	// The original is no longer in the current draft or normal media library.
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=now(),purge_after=now()+interval '30 days',revision=revision+1,update_time=now() WHERE id=?`, document).Error; err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, pid).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	store := workspaceWithActualScript(db)
	job, err := store.Create(t.Context(), actor, workspaceapp.ProjectCopyInput{SourceProjectID: pid, ExpectedRevision: revision, TargetName: "完整来源历史", IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}, time.Now().UTC())
	if err != nil {
		t.Fatal("all retained history freeze", err)
	}
	if job.Manifest.Assets != 2 || job.Manifest.Script.Counts.Sources != 2 || job.Manifest.Script.Counts.Versions != 2 {
		t.Fatal("omitted retired original/history", job.Manifest)
	}
	worker := uuid.New()
	job, err = store.Claim(t.Context(), actor, job.ID, worker, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	mb := mediaapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: pid, TargetProjectID: job.TargetProjectID}
	ms := mediaapp.ProjectCopySnapshot{ID: job.Manifest.MediaSnapshotID, ManifestSHA256: job.Manifest.MediaSHA256, Assets: job.Manifest.Assets, Renditions: job.Manifest.Renditions}
	mt := mediaapp.NewProjectCopyTransfer(workspacepg.NewProjectCopyMediaObjects(store, job.ID, worker, false), mediaobjects.NewProjectCopyObjects(storage), t.TempDir())
	if err := mt.Transfer(t.Context(), actor, mb, ms); err != nil {
		t.Fatal("actual private document transfer", err)
	}
	job, err = store.CompleteMedia(t.Context(), actor, job.ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	transfer, b, snapshot := scriptCopyTransfer(db, job, worker, false, objects)
	if _, err := transfer.Transfer(t.Context(), actor, b, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteScript(t.Context(), actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteCanvases(t.Context(), actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(t.Context(), actor, job.ID, worker); err != nil {
		t.Fatal("whole job publication", err)
	}
	var target struct {
		ID           uuid.UUID
		MediaAssetID uuid.UUID
		OriginalKey  string
		Provenance   string
	}
	if err := owner.Raw(`SELECT id,media_asset_id,original_key,provenance::text FROM script.script_source WHERE project_id=? AND original_sha256=?`, job.TargetProjectID, domain.ContentSHA(word)).Scan(&target).Error; err != nil || target.ID == uuid.Nil || target.MediaAssetID == document || target.MediaAssetID == uuid.Nil {
		t.Fatal("historical source remapping", target, err)
	}
	var provenance domain.SourceProvenance
	if err := json.Unmarshal([]byte(target.Provenance), &provenance); err != nil || len(provenance.Warnings) != 1 || provenance.Warnings[0].Code != "table_layout_flattened" {
		t.Fatal("copy discarded formal warnings", provenance, err)
	}
	media, err := mediapg.NewStore(db).FindAsset(t.Context(), actor, job.TargetProjectID, target.MediaAssetID)
	if err != nil || media.IsDelete {
		t.Fatal("retained original not independently ready", media, err)
	}
	// Deleting only synthetic source bytes cannot affect either private target copy.
	if err := objects.Remove(t.Context(), old.Sources[0].Original.Key); err != nil {
		t.Fatal(err)
	}
	var originalMediaKey string
	if err := owner.Raw(`SELECT object_key FROM media.media_asset WHERE id=?`, document).Scan(&originalMediaKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := storage.Remove(t.Context(), originalMediaKey); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{target.OriginalKey, media.ObjectKey} {
		r, err := storage.Get(t.Context(), key)
		if err != nil {
			t.Fatal("target borrowed source object", err)
		}
		actual, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(actual, word) {
			t.Fatal("target lost exact original DOCX", readErr, closeErr)
		}
	}
	current, err := scriptStore(db).LoadBase(t.Context(), actor, job.TargetProjectID, nil)
	if err != nil || len(current.Sources) != 1 || current.Sources[0].Provenance.Encoding != "utf-16le" {
		t.Fatal("current UTF16 draft lost", current, err)
	}
	var importJobs int64
	if err := owner.Raw(`SELECT count(*) FROM script.import_job WHERE project_id=?`, job.TargetProjectID).Scan(&importJobs).Error; err != nil || importJobs != 0 {
		t.Fatal("copied execution history", importJobs, err)
	}
}
