package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestMediaPurgeCancelBeforeIORestoresTrashBytesAndRejectsNewWorkWhileReserved(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project, in, keys, objects := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: in.Scope, Key: uuid.New(), ExpectedRevision: 3, Action: "restore_items", Items: in.Items}); !errors.Is(err, domain.ErrPurgeConflict) {
		t.Fatal("new metadata restore bypassed physical reservation", err)
	}
	target := uuid.New()
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'reservation fixture','16:9','realistic','copying')`, target, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = pgmedia.NewProjectCopyStore(db).FreezeWithReferences(t.Context(), actor, mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: target}, time.Now(), []uuid.UUID{in.Items[0].ID})
	if !errors.Is(err, domain.ErrPurgeConflict) {
		t.Fatal("history admission ignored purge reservation", err)
	}
	key := uuid.New()
	cancelled, err := repo.ControlPurge(t.Context(), actor, job.ID, key, job.Revision, "cancel")
	if err != nil || cancelled.Status != "cancelled" || !cancelled.CancellationRequested {
		t.Fatal("cancel unstarted actual deletion", cancelled, err)
	}
	for _, key := range keys {
		present, err := objects.Exists(t.Context(), key)
		if err != nil || !present {
			t.Fatal("cancel deleted source bytes", err)
		}
	}
	detail, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).LibraryDetail(t.Context(), actor, in.Scope, in.Items[0].ID)
	if err != nil || detail.State != "trashed" {
		t.Fatal("cancel restored the asset to active instead of its original trash", detail, err)
	}
	replay, err := repo.ControlPurge(t.Context(), actor, job.ID, key, job.Revision, "cancel")
	want, _ := json.Marshal(cancelled)
	actual, _ := json.Marshal(replay)
	if err != nil || !bytes.Equal(want, actual) {
		t.Fatal("permanent cancel replay changed", err)
	}
}

func TestMediaPurgeActualPersonalTextScrubsContentAndNoObjectsAreInvented(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	text := "真实待清理正文"
	created, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "私有名称", Tags: []string{"私有标签"}, Note: "私有备注", SourceLabel: "私有来源", Category: "other"}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Items[0].ID
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 1, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: id, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, nil, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, mediaapp.PurgeInput{Scope: scope, Items: []mediaapp.LibraryItemRevision{{ID: id, Revision: 2}}, ExpectedRevision: 2, PermanentDeleteConfirmed: true, Key: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	objects := &purgePhysicalFixture{objects: map[string][]byte{}, started: map[string]bool{}, removed: map[string]bool{}}
	complete, err := mediaapp.NewPurgeWorker(repo, objects).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil || complete.Status != "succeeded" || objects.deleteCalls != 0 {
		t.Fatal("text fabricated a physical object", complete, err)
	}
	var row struct {
		PlainText, Title, Tags, SourceLabel, Note string
		Purged                                    bool
	}
	if err := db.Raw(`SELECT plain_text,title,tags::text AS tags,source_label,note,purged_at IS NOT NULL AS purged FROM media.library_item WHERE id=?`, id).Scan(&row).Error; err != nil || row.PlainText != "" || row.Title != "已永久清理" || row.Tags != "[]" || row.SourceLabel != "" || row.Note != "" || !row.Purged {
		t.Fatal("private text survived in mutable catalog", row, err)
	}
	q := libraryQuery()
	q.State = "trashed"
	page, err := library.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || page.Total != 0 {
		t.Fatal("purged tombstone entered full query", page, err)
	}
}

func TestMediaPurgeZeroRowPermanentReceiptRollsBackRawDeletionAndReservation(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, project, in, _, _ := purgeBinaryFixture(t, db)
	name := "purge_zero_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ddl := `CREATE FUNCTION media.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.actor_id='` + actor.ID.String() + `'::uuid THEN RETURN NULL; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + name + ` BEFORE INSERT ON media.purge_command FOR EACH ROW EXECUTE FUNCTION media.` + name + `() `
	if err := owner.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + name + ` ON media.purge_command; DROP FUNCTION media.` + name + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	if _, err := repo.CreatePurge(t.Context(), actor, in); !errors.Is(err, mediaapp.ErrLibraryConflict) {
		t.Fatal("zero-row permanent receipt was treated as a committed mutation", err)
	}
	var deleted bool
	if err := db.Raw(`SELECT is_delete FROM media.media_asset WHERE project_id=? AND id=?`, project, in.Items[0].ID).Scan(&deleted).Error; err != nil || deleted {
		t.Fatal("receipt failure left source deleted", deleted, err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM media.purge_job WHERE actor_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("rollback left occupied jobs", count, err)
	}
	if err := owner.Exec(`DROP TRIGGER ` + name + ` ON media.purge_command`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE INSERT ON media.purge_command FOR EACH ROW WHEN (false) EXECUTE FUNCTION media.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreatePurge(t.Context(), actor, in); err != nil {
		t.Fatal("rolled-back original key could not be accepted", err)
	}
}

func TestMediaPurgeExpiredFenceCannotStealOrPublishAndRevocationStillAllowsExactEnd(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _, in, keys, objects := purgeBinaryFixture(t, db)
	now := time.Now()
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, func() time.Time { return now })
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	work := mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()}
	lease, err := repo.ClaimPurge(t.Context(), work)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	other := work
	other.ExecutionID = uuid.NewString()
	if _, err := repo.ClaimPurge(t.Context(), other); !errors.Is(err, domain.ErrPurgeConflict) {
		t.Fatal("expired lease stole an unended physical owner", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPurge(t.Context(), actor, job.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked actor read private cleanup", err)
	}
	if _, err := repo.ControlPurge(t.Context(), actor, job.ID, uuid.New(), job.Revision, "cancel"); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked actor controlled cleanup", err)
	}
	if err := repo.EndPurgePhysical(context.Background(), lease); err != nil {
		t.Fatal("exact cessation fact incorrectly required current permissions", err)
	}
	for _, key := range keys {
		present, err := objects.Exists(t.Context(), key)
		if err != nil || !present {
			t.Fatal("blocked owner physically deleted a source", err)
		}
	}
}

func TestMediaPurgeMissingIntentRowsCannotFabricateCompleteDeletion(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, _, in, _, _ := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := repo.ClaimPurge(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StartPurgeItem(t.Context(), lease, 0); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`DELETE FROM media.purge_object WHERE job_id=?`, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.CompletePurgeItem(t.Context(), lease, 0); !errors.Is(err, domain.ErrPurgeConflict) {
		t.Fatal("zero surviving proof rows falsely proved absence", err)
	}
}

func TestMediaPurgeUnknownRemovalCancelRetainsPurposeAndFinishesOnlyOriginalObjects(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _, in, keys, objects := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	unknown := &purgeUnknownRemove{PurgeObjects: objects, once: true}
	current, err := mediaapp.NewPurgeWorker(repo, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil || current.Status != "needs_reconciliation" {
		t.Fatal("unknown original outcome was not retained", current, err)
	}
	key := uuid.New()
	cancelled, err := repo.ControlPurge(t.Context(), actor, job.ID, key, current.Revision, "cancel")
	if err != nil || cancelled.Status != "cancel_requested" || !cancelled.CancellationRequested {
		t.Fatal("unknown removal falsely became cancelled or lost cancellation purpose", cancelled, err)
	}
	complete, err := mediaapp.NewPurgeWorker(repo, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil || complete.Status != "succeeded" || !complete.CancellationRequested || unknown.removeCount != len(keys) {
		t.Fatal("started irreversible cleanup did not finish only its original object set", complete, err, unknown.removeCount)
	}
	for _, key := range keys {
		present, err := objects.Exists(t.Context(), key)
		if err != nil || present {
			t.Fatal("cleanup completion retained a real original or rendition", err)
		}
	}
	replay, err := repo.ControlPurge(t.Context(), actor, job.ID, key, current.Revision, "cancel")
	want, _ := json.Marshal(cancelled)
	actual, _ := json.Marshal(replay)
	if err != nil || !bytes.Equal(want, actual) {
		t.Fatal("later cleanup rewrote permanent cancel response", err)
	}
	if _, err := repo.ControlPurge(t.Context(), actor, job.ID, key, current.Revision+1, "cancel"); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("different cancel body reused a permanent key", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ControlPurge(t.Context(), actor, job.ID, key, current.Revision, "cancel"); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked actor replayed a private permanent response", err)
	}
}

func TestMediaPurgeControlZeroRowReceiptsRollbackCancelAndReconcile(t *testing.T) {
	for _, action := range []string{"cancel", "reconcile"} {
		t.Run(action, func(t *testing.T) {
			db := libraryRuntimeDB(t)
			owner := libraryOwnerDB(t)
			actor, _, in, _, objects := purgeBinaryFixture(t, db)
			repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
			job, err := repo.CreatePurge(t.Context(), actor, in)
			if err != nil {
				t.Fatal(err)
			}
			if action == "reconcile" {
				job, err = mediaapp.NewPurgeWorker(repo, &purgeUnknownRemove{PurgeObjects: objects, once: true}).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
				if err != nil || job.Status != "needs_reconciliation" {
					t.Fatal("unknown fixture", job, err)
				}
			}
			name := "purge_control_zero_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			ddl := `CREATE FUNCTION media.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.actor_id='` + actor.ID.String() + `'::uuid THEN RETURN NULL; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + name + ` BEFORE INSERT ON media.purge_command FOR EACH ROW EXECUTE FUNCTION media.` + name + `() `
			if err := owner.Exec(ddl).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := owner.Exec(`DROP TRIGGER IF EXISTS ` + name + ` ON media.purge_command; DROP FUNCTION media.` + name + `()`).Error; err != nil {
					t.Error(err)
				}
			})
			key := uuid.New()
			if _, err := repo.ControlPurge(t.Context(), actor, job.ID, key, job.Revision, action); !errors.Is(err, mediaapp.ErrLibraryConflict) {
				t.Fatal("zero-row permanent control was accepted", err)
			}
			actual, err := repo.GetPurge(t.Context(), actor, job.ID)
			wantJSON, _ := json.Marshal(job)
			actualJSON, _ := json.Marshal(actual)
			if err != nil || !bytes.Equal(wantJSON, actualJSON) {
				t.Fatal("failed permanent control changed attempt or cancellation facts", actual, err)
			}
			var raw struct {
				IsDelete bool
				Revision int64
			}
			if err := db.Raw(`SELECT is_delete,revision FROM media.media_asset WHERE id=?`, in.Items[0].ID).Scan(&raw).Error; err != nil || !raw.IsDelete || raw.Revision != 2 {
				t.Fatal("receipt rollback did not retain the reserved original", raw, err)
			}
			if err := owner.Exec(`DROP TRIGGER ` + name + ` ON media.purge_command`).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ControlPurge(t.Context(), actor, job.ID, key, job.Revision, action); err != nil {
				t.Fatal("rolled-back original control key could not be used", err)
			}
		})
	}
}
