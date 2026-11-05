package media_test

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/storagefixture"
)

func TestMediaTransferQueuedCancellationIsPermanentAndNeverPublishesText(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project := mediaStoreProject(t, db)
	source := domain.LibraryScope{Kind: domain.LibraryPersonal}
	target := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	text := "真实纯文本，不包含对象"
	created, err := mediaLibraryStore(db, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: source, Action: "create_text", Key: uuid.New(), Metadata: &mediaapp.LibraryMetadata{Title: "等待转移", Category: "other", PlainText: &text}})
	if err != nil || len(created.Items) != 1 {
		t.Fatal(err)
	}
	repo := mediaTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
	job, err := repo.CreateTransfer(t.Context(), actor, mediaapp.TransferInput{Source: source, Target: target, Key: uuid.New(), ExpectedSourceRevision: 1, ExpectedProjectRevision: 1, Items: []mediaapp.LibraryItemRevision{{ID: created.Items[0].ID, Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	cancelled, err := repo.ControlTransfer(t.Context(), actor, job.ID, key, job.Revision, "cancel")
	if err != nil || cancelled.Status != "cancelled" || cancelled.Items[0].Status != "cancelled" || cancelled.ExecutionUnconfirmed || cancelled.NeedsReconciliation {
		t.Fatal("queued SQL-only cancellation fabricated physical work", cancelled, err)
	}
	replay, err := repo.ControlTransfer(t.Context(), actor, job.ID, key, job.Revision, "cancel")
	if err != nil || !reflect.DeepEqual(replay, cancelled) {
		t.Fatal("permanent cancellation replay changed", replay, err)
	}
	if _, err := repo.ControlTransfer(t.Context(), actor, job.ID, key, job.Revision+1, "cancel"); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("same cancellation key accepted another input", err)
	}
	page, err := mediaLibraryStore(db, libraryTestAccess, time.Now).ListLibrary(t.Context(), actor, target, libraryQuery())
	if err != nil || page.Total != 0 {
		t.Fatal("cancelled unpublished text leaked into target", page, err)
	}
	if _, err := repo.ControlTransfer(t.Context(), actor, job.ID, uuid.New(), cancelled.Revision, "retry"); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("cancelled intent silently restarted", err)
	}
	foreign, _ := mediaStoreProject(t, db)
	if _, err := repo.ControlTransfer(t.Context(), foreign, job.ID, key, job.Revision, "cancel"); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("foreign actor replayed creator's receipt", err)
	}
}

func TestMediaTransferQueuedBinaryCancellationDrainsEveryPhysicalReservation(t *testing.T) {
	t.Setenv("GLOWLOOM_TEST_STORAGE_ROUTE_DSN", os.Getenv("GLOWLOOM_TEST_STORAGE_GATE_DSN"))
	db := storageRouteDB(t).Begin()
	if db.Error != nil {
		t.Fatal(db.Error)
	}
	t.Cleanup(func() { _ = db.Rollback().Error })
	if err := db.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	var role string
	if err := db.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("runtime role required", role, err)
	}
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	f := newTransferRecoveryFixtureOn(t, db, 2)
	key := uuid.New()
	requested, err := f.repo.ControlTransfer(t.Context(), f.actor, f.job.ID, key, f.job.Revision, mediaapp.TransferActionCancel)
	if err != nil || requested.Status != domain.TransferStatusCancelRequested || !requested.CancellationRequested {
		t.Fatal("queued physical reservations were declared cancelled before cleanup", requested, err)
	}
	objects := &transferWriteAudit{PhysicalTransferObjects: mediaFixtureCopyObjects(t, db), writes: map[domain.ObjectRef]int{}, absenceChecks: map[domain.ObjectRef]int{}}
	result, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()).Execute(t.Context(), transferExecution(requested))
	if err != nil || result.Status != domain.TransferStatusCancelled || result.NeedsReconciliation || result.ExecutionUnconfirmed {
		t.Fatal("queued binary cancellation did not converge", result, err)
	}
	if len(objects.absenceChecks) != 6 {
		t.Fatal("worker did not inspect every exact physical target", objects.absenceChecks)
	}
	if len(objects.writes) != 0 {
		t.Fatal("cancellation wrote unpublished output", objects.writes)
	}
	for _, item := range result.Items {
		if item.Status != domain.TransferItemStatusCancelled {
			t.Fatal("cancellation skipped a future item", item)
		}
	}
	var proof struct{ Objects, Removed, Deleted, CancelledWork int64 }
	if err := db.Raw(`SELECT count(*) AS objects,count(*) FILTER (WHERE o.status='removed') AS removed,count(*) FILTER (WHERE l.state='deleted') AS deleted,count(DISTINCT w.id) FILTER (WHERE w.state='cancelled') AS cancelled_work FROM media.transfer_object o JOIN media.object_location l ON l.id=o.target_location_id JOIN media.storage_work_intent w ON w.id=l.owner_intent_id WHERE o.job_id=?`, f.job.ID).Scan(&proof).Error; err != nil || proof.Objects != 6 || proof.Removed != 6 || proof.Deleted != 6 || proof.CancelledWork != 2 {
		t.Fatal("cancelled output facts retain physical debt", proof, err)
	}
	for _, fact := range transferOutputFacts(t, db, f.job.ID) {
		if exists, err := f.objects.Exists(t.Context(), fact.TargetObjectKey); err != nil || exists {
			t.Fatal("queued cancellation retained target bytes", fact, exists, err)
		}
	}
	var published int64
	if err := db.Table("media.media_asset").Where("id IN ?", []uuid.UUID{*result.Items[0].TargetAssetID, *result.Items[1].TargetAssetID}).Count(&published).Error; err != nil || published != 0 {
		t.Fatal("cancelled binary was published", published, err)
	}
	if replay, err := f.repo.ControlTransfer(t.Context(), f.actor, f.job.ID, key, f.job.Revision, mediaapp.TransferActionCancel); err != nil || !reflect.DeepEqual(replay, requested) {
		t.Fatal("cancelled receipt changed after physical cleanup", replay, err)
	}
	environment := storageEnvironmentFixture(t, db, "queued-cancel-readonly-"+uuid.NewString(), "cos-main")
	for i := range environment.Profiles {
		if environment.Profiles[i].ID == storagefixture.Configured(t).DefaultWriteProfile {
			environment.Profiles[i].Mode = mediaapp.StorageProfileModeReadOnly
		}
	}
	environment.ConfigSHA256 = ""
	raw, err := json.Marshal(environment)
	if err != nil {
		t.Fatal(err)
	}
	environment.ConfigSHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	if _, err := routes.ApplyEnvironment(t.Context(), environment); err != nil {
		t.Fatal("fully cancelled work still blocked readonly route", err)
	}
}
