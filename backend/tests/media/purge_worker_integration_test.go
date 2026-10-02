package media_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func purgeBinaryFixture(t *testing.T, db *gorm.DB) (identityapp.Principal, uuid.UUID, mediaapp.PurgeInput, []string, mediaapp.PurgeObjects) {
	t.Helper()
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	result := documentUpload(t, db, objects, actor, project, "purge-private.png", uploadPNG(t))
	var keys []string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION ALL SELECT object_key FROM media.rendition WHERE media_asset_id=?`, result.Asset.ID, result.Asset.ID).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = objects.Remove(context.Background(), key)
		}
	})
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "update_item", ItemID: &result.Asset.ID, Metadata: &mediaapp.LibraryMetadata{Title: "仅合成测试原件", Category: "material", Tags: []string{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "recycle_items", ExpectedRevision: 1, Items: []mediaapp.LibraryItemRevision{{ID: result.Asset.ID, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	return actor, project, mediaapp.PurgeInput{Scope: scope, Items: []mediaapp.LibraryItemRevision{{ID: result.Asset.ID, Revision: 2}}, ExpectedRevision: 2, ExpectedProjectRevision: 3, PermanentDeleteConfirmed: true, Key: uuid.New()}, keys, copyobjects.NewProjectCopyObjects(objects)
}

func TestMediaPurgeActualOriginalAndEveryRenditionAbsentBeforeTombstone(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project, in, keys, objects := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	work := mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()}
	completed, err := mediaapp.NewPurgeWorker(repo, objects).Execute(t.Context(), work)
	if err != nil || completed.Status != "succeeded" || completed.Items[0].Status != "succeeded" {
		t.Fatal("real private-object cleanup", completed, err)
	}
	for _, key := range keys {
		present, err := objects.Exists(t.Context(), key)
		if err != nil || present {
			t.Fatal("declared success while actual original or rendition exists", key, err)
		}
	}
	var row struct {
		Title, PlainText, SourceLabel, Note string
		Purged                              bool
		CatalogState                        string
	}
	if err := db.Raw(`SELECT title,COALESCE(plain_text,'') AS plain_text,source_label,note,purged_at IS NOT NULL AS purged,catalog_state FROM media.library_item WHERE id=?`, in.Items[0].ID).Scan(&row).Error; err != nil || !row.Purged || row.Title != "已永久清理" || row.CatalogState != "removed" || row.PlainText != "" || row.Note != "" {
		t.Fatal("visible title/content was not scrubbed", row, err)
	}
	if _, err := pgmedia.NewStore(db).FindAsset(t.Context(), actor, project, in.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("purged original became formally readable", err)
	}
	repeated, err := mediaapp.NewPurgeWorker(repo, objects).Execute(t.Context(), work)
	if err != nil || repeated.Revision != completed.Revision {
		t.Fatal("duplicate physical delivery changed terminal result", repeated, err)
	}
}

type purgeUnknownRemove struct {
	mediaapp.PurgeObjects
	once        bool
	removeCount int
}

func (p *purgeUnknownRemove) Remove(ctx context.Context, key string) error {
	p.removeCount++
	if err := p.PurgeObjects.Remove(ctx, key); err != nil {
		return err
	}
	if p.once {
		p.once = false
		return io.ErrUnexpectedEOF
	}
	return nil
}

func TestMediaPurgeActualUnknownDeleteReconcileOriginalIntentAndPermanentKey(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _, in, keys, objects := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	unknown := &purgeUnknownRemove{PurgeObjects: objects, once: true}
	current, err := mediaapp.NewPurgeWorker(repo, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil || current.Status != "needs_reconciliation" || !current.NeedsReconciliation || unknown.removeCount != 1 {
		t.Fatal("unknown actual deletion was not retained", current, err)
	}
	var purged bool
	if err := db.Raw(`SELECT purged_at IS NOT NULL FROM media.library_item WHERE id=?`, in.Items[0].ID).Scan(&purged).Error; err != nil || purged {
		t.Fatal("unknown cleanup falsely scrubbed full catalog", err)
	}
	key := uuid.New()
	queued, err := repo.ControlPurge(t.Context(), actor, job.ID, key, current.Revision, "reconcile")
	if err != nil || queued.Attempt != 2 || queued.Status != "queued" {
		t.Fatal("exact original cleanup could not resume", queued, err)
	}
	complete, err := mediaapp.NewPurgeWorker(repo, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 2, ExecutionID: uuid.NewString()})
	if err != nil || complete.Status != "succeeded" || unknown.removeCount != len(keys) {
		t.Fatal("absence receipt retried a destructively completed key", complete, err, unknown.removeCount)
	}
	original, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil || original.ID != job.ID || original.Revision != job.Revision {
		t.Fatal("newer cleanup revised original admission replay", original, err)
	}
	replay, err := repo.ControlPurge(t.Context(), actor, job.ID, key, current.Revision, "reconcile")
	if err != nil || replay.Revision != queued.Revision {
		t.Fatal("later physical execution broke permanent reconcile result", replay, err)
	}
}

type purgeFailureReceiptFault struct {
	mediaapp.PurgeWorkerRepository
	err error
}

func (p purgeFailureReceiptFault) FailPurgeItem(context.Context, mediaapp.PurgeLease, int, string) error {
	return p.err
}

func TestMediaPurgeFailureReceiptErrorIsReturnedAfterActualPhysicalOwnerEnds(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _, in, _, objects := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("owning failure receipt unavailable")
	fault := purgeFailureReceiptFault{PurgeWorkerRepository: repo, err: want}
	unknown := &purgeUnknownRemove{PurgeObjects: objects, once: true}
	current, err := mediaapp.NewPurgeWorker(fault, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if !errors.Is(err, want) {
		t.Fatal("failed durable receipt was silently discarded", current, err)
	}
	var ended bool
	if err := db.Raw(`SELECT process_ended FROM media.purge_job WHERE id=?`, job.ID).Scan(&ended).Error; err != nil || !ended {
		t.Fatal("error returned without actual physical owner cessation", ended, err)
	}
	actual, err := repo.GetPurge(t.Context(), actor, job.ID)
	if err != nil || actual.Status != "needs_reconciliation" {
		t.Fatal("unrecorded failure lost its occupied original intent", actual, err)
	}
}
