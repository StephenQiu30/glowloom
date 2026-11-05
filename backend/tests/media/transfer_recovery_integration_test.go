package media_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type transferRecoveryFixture struct {
	db      *gorm.DB
	objects *objectstorage.Client
	repo    *mediaTransferStores
	actor   identityapp.Principal
	input   mediaapp.TransferInput
	job     domain.TransferJob
}

func newTransferRecoveryFixture(t *testing.T, count int) transferRecoveryFixture {
	t.Helper()
	return newTransferRecoveryFixtureOn(t, libraryRuntimeDB(t), count)
}

func newTransferRecoveryFixtureOn(t *testing.T, db *gorm.DB, count int) transferRecoveryFixture {
	t.Helper()
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	base := mediaTestStore(db)
	uploads := mediaFixtureRoutedUpload(t, db, base, base, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{})
	input := mediaapp.TransferInput{Source: domain.LibraryScope{Kind: domain.LibraryPersonal}, Target: domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, ExpectedSourceRevision: int64(count), ExpectedProjectRevision: 1, Key: uuid.New()}
	for index := range count {
		in := uploadInput(t)
		img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		img.SetNRGBA(0, 0, color.NRGBA{R: uint8(index + 1), A: 255})
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			t.Fatal(err)
		}
		file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(encoded.Bytes()), "转移.png")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		in.File, in.Request.FileName = file, "转移.png"
		in.Actor, in.Request.ProjectID = actor, uuid.Nil
		result, err := fixturePersonalUpload(t.Context(), uploads, in)
		if err != nil {
			t.Fatal(err)
		}
		input.Items = append(input.Items, mediaapp.LibraryItemRevision{ID: result.Asset.ID, Revision: 1})
	}
	repo := mediaTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
	job, err := repo.CreateTransfer(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var keys []string
		err := db.WithContext(ctx).Raw(`SELECT source_object_key FROM media.transfer_object WHERE job_id=? UNION SELECT target_object_key FROM media.transfer_object WHERE job_id=?`, job.ID, job.ID).Scan(&keys).Error
		if err != nil {
			t.Error("read exact synthetic object cleanup keys", err)
			return
		}
		for _, key := range keys {
			if err := objects.Remove(ctx, key); err != nil {
				t.Error("remove synthetic transfer object", err)
			}
		}
	})
	return transferRecoveryFixture{db: db, objects: objects, repo: repo, actor: actor, input: input, job: job}
}

type transferUnknownWrite struct {
	mediaapp.PhysicalTransferObjects
	writes, failAt int
}

func (o *transferUnknownWrite) PutIfAbsent(ctx context.Context, key domain.ObjectRef, reader io.Reader, size int64, mime, digest string) error {
	err := o.PhysicalTransferObjects.PutIfAbsent(ctx, key, reader, size, mime, digest)
	o.writes++
	if err == nil && o.writes == o.failAt {
		return io.ErrUnexpectedEOF
	}
	return err
}

func transferExecution(job domain.TransferJob) mediaapp.TransferWorkID {
	return mediaapp.TransferWorkID{JobID: job.ID, Attempt: job.Attempt, ExecutionID: uuid.NewString()}
}

func TestMediaTransferUnknownWriteReconcilesExactObjectsAndPermanentReceipts(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	objects := &transferUnknownWrite{PhysicalTransferObjects: mediaFixtureCopyObjects(t, f.db), failAt: 1}
	unknown, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()).Execute(fixtureCopyContext(t), transferExecution(f.job))
	if err != nil || unknown.Status != "needs_reconciliation" || !unknown.NeedsReconciliation || unknown.ExecutionUnconfirmed || unknown.Items[0].Status != "needs_reconciliation" {
		t.Fatal("unknown physical write was treated as terminal", unknown, err)
	}
	if _, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, uuid.New(), unknown.Revision, "retry"); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("unsafe unknown write was automatically retried", err)
	}
	physicalBefore := transferOutputFacts(t, f.db, f.job.ID)
	key := uuid.New()
	reconcile, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "reconcile")
	if err != nil || reconcile.Attempt != 2 || reconcile.Items[0].TargetItemID != unknown.Items[0].TargetItemID {
		t.Fatal("reconciliation changed independent target identity", reconcile, err)
	}
	if physicalAfter := transferOutputFacts(t, f.db, f.job.ID); !reflect.DeepEqual(physicalAfter, physicalBefore) {
		t.Fatal("reconciliation allocated a new physical attempt", physicalBefore, physicalAfter)
	}
	result, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()).Execute(t.Context(), transferExecution(reconcile))
	if err != nil || result.Status != "succeeded" || result.NeedsReconciliation || objects.writes != 3 {
		t.Fatal("actual existing original was not read back before remaining renditions", result, objects.writes, err)
	}
	replay, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "reconcile")
	if err != nil || !reflect.DeepEqual(replay, reconcile) {
		t.Fatal("original recovery receipt was rewritten by later progress", replay, err)
	}
	if _, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision+1, "reconcile"); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("permanent recovery key accepted a different body", err)
	}
	original, err := f.repo.CreateTransfer(t.Context(), f.actor, f.input)
	if err != nil || !reflect.DeepEqual(original, f.job) {
		t.Fatal("original admission receipt changed after completion", original, err)
	}
}

func TestMediaTransferCancellationCleansUnknownObjectsAndPreservesPublishedRows(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(map[int]string{1: "unknown only", 2: "published row and unknown row"}[count], func(t *testing.T) {
			f := newTransferRecoveryFixture(t, count)
			objects := &transferUnknownWrite{PhysicalTransferObjects: mediaFixtureCopyObjects(t, f.db), failAt: (count-1)*3 + 1}
			unknown, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()).Execute(fixtureCopyContext(t), transferExecution(f.job))
			if err != nil || unknown.Status != "needs_reconciliation" {
				t.Fatal(unknown, err)
			}
			key := uuid.New()
			requested, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "cancel")
			if err != nil || !requested.CancellationRequested || requested.Status != "cancel_requested" {
				t.Fatal("cancel intent was not retained independently", requested, err)
			}
			drainFixtureTransfer(t, libraryOwnerDB(t), f.job.ID)
			result, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()).Execute(t.Context(), transferExecution(requested))
			wantStatus := map[int]domain.TransferStatus{1: domain.TransferStatusCancelled, 2: domain.TransferStatusPartialFailed}[count]
			if err != nil || result.Status != wantStatus || !result.CancellationRequested || result.NeedsReconciliation || result.ExecutionUnconfirmed || result.Items[count-1].Status != "cancelled" || objects.writes != (count-1)*3+1 {
				t.Fatal("cleanup did not converge or cancellation resumed writes", result, objects.writes, err)
			}
			var rows []struct {
				ItemIndex       int
				TargetObjectKey string
			}
			if err := f.db.Raw(`SELECT item_index,target_object_key FROM media.transfer_object WHERE job_id=?`, result.ID).Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				exists, err := f.objects.Exists(t.Context(), row.TargetObjectKey)
				if err != nil || exists != (row.ItemIndex < count-1) {
					t.Fatal("cleanup touched published output or retained unpublished output", row.ItemIndex, exists, err)
				}
			}
			replay, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "cancel")
			if err != nil || !reflect.DeepEqual(replay, requested) {
				t.Fatal("cancel receipt changed after cleanup", replay, err)
			}
			if _, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision+1, "cancel"); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
				t.Fatal("cancel key accepted another revision", err)
			}
			if _, err := f.repo.ControlTransfer(t.Context(), f.actor, result.ID, uuid.New(), result.Revision, "retry"); !errors.Is(err, domain.ErrTransferConflict) {
				t.Fatal("permanent cancellation resumed copying", err)
			}
		})
	}
}

type transferShortWriteFence struct {
	mediaapp.TransferWorkerRepository
	short      bool
	firstLease *mediaapp.TransferLease
}

func (r *transferShortWriteFence) ClaimTransfer(ctx context.Context, id mediaapp.TransferWorkID) (mediaapp.TransferLease, error) {
	lease, err := r.TransferWorkerRepository.ClaimTransfer(ctx, id)
	if err == nil && r.firstLease == nil {
		r.firstLease = &lease
	}
	return lease, err
}

func (r *transferShortWriteFence) BeginTransferWrite(ctx context.Context, lease mediaapp.TransferLease, index int, kind string) error {
	if !r.short {
		return r.TransferWorkerRepository.BeginTransferWrite(ctx, lease, index, kind)
	}
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return r.TransferWorkerRepository.BeginTransferWrite(bounded, lease, index, kind)
}

type transferWriteAudit struct {
	mediaapp.PhysicalTransferObjects
	writes        map[domain.ObjectRef]int
	absenceChecks map[domain.ObjectRef]int
}

func (o *transferWriteAudit) PutIfAbsent(ctx context.Context, ref domain.ObjectRef, body io.Reader, size int64, mime, digest string) error {
	o.writes[ref]++
	return o.PhysicalTransferObjects.PutIfAbsent(ctx, ref, body, size, mime, digest)
}

func (o *transferWriteAudit) Exists(ctx context.Context, ref domain.ObjectRef) (bool, error) {
	if o.absenceChecks != nil {
		o.absenceChecks[ref]++
	}
	return o.PhysicalTransferObjects.Exists(ctx, ref)
}

type transferOutputFact struct {
	ItemIndex, OutputAttempt                          int
	RenditionKind, TargetObjectKey, ProfileID, Bucket string
	TargetLocationID                                  uuid.UUID
}

func transferOutputFacts(t *testing.T, db *gorm.DB, job uuid.UUID) []transferOutputFact {
	t.Helper()
	var rows []transferOutputFact
	if err := db.Raw(`SELECT o.item_index,o.output_attempt,o.rendition_kind,o.target_object_key,o.target_location_id,l.profile_id,l.bucket FROM media.transfer_object o JOIN media.object_location l ON l.id=o.target_location_id WHERE o.job_id=? ORDER BY o.item_index,o.output_attempt,o.rendition_kind`, job).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

type transferExpiredCleanupObjects struct {
	mediaapp.PhysicalTransferObjects
	missing string
	expire  func()
	seeded  bool
}

func (o *transferExpiredCleanupObjects) expireMissing(ref domain.ObjectRef, err error) {
	if errors.Is(err, objectstorage.ErrObjectMissing) && ref.Key == o.missing && !o.seeded {
		o.seeded = true
		o.expire()
	}
}
func (o *transferExpiredCleanupObjects) Get(ctx context.Context, ref domain.ObjectRef) (io.ReadCloser, error) {
	reader, err := o.PhysicalTransferObjects.Get(ctx, ref)
	o.expireMissing(ref, err)
	if err != nil {
		return nil, err
	}
	return &transferExpiredCleanupReader{ReadCloser: reader, objects: o, ref: ref}, nil
}

type transferExpiredCleanupReader struct {
	io.ReadCloser
	objects *transferExpiredCleanupObjects
	ref     domain.ObjectRef
}

func (r *transferExpiredCleanupReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.objects.expireMissing(r.ref, err)
	return n, err
}

func TestMediaTransferPartialFailureCleansOnlyFailedItemAndRetriesSameIdentity(t *testing.T) {
	for _, missingKind := range []string{"", "thumb_256"} {
		t.Run(map[string]string{"": "missing original before write", "thumb_256": "missing rendition after original write"}[missingKind], func(t *testing.T) {
			f := newTransferRecoveryFixture(t, 2)
			var missing string
			if err := f.db.Raw(`SELECT source_object_key FROM media.transfer_object WHERE job_id=? AND item_index=1 AND rendition_kind=?`, f.job.ID, missingKind).Scan(&missing).Error; err != nil || missing == "" {
				t.Fatal(err)
			}
			reader, err := f.objects.Get(t.Context(), missing)
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(reader)
			if err := errors.Join(readErr, reader.Close()); err != nil {
				t.Fatal(err)
			}
			if err := f.objects.Remove(t.Context(), missing); err != nil {
				t.Fatal(err)
			}
			audit := &transferWriteAudit{PhysicalTransferObjects: mediaFixtureCopyObjects(t, f.db), writes: map[domain.ObjectRef]int{}}
			var objects mediaapp.PhysicalTransferObjects = audit
			workerRepo := &transferShortWriteFence{TransferWorkerRepository: f.repo.Worker, short: missingKind != ""}
			if missingKind != "" {
				// Seed an aged, ended cancellation exactly once, before production cleanup
				// first closes this intent. Durable closed_at/write_until are never regressed.
				objects = &transferExpiredCleanupObjects{PhysicalTransferObjects: objects, missing: missing, expire: func() { drainFixtureTransfer(t, libraryOwnerDB(t), f.job.ID) }}
			}
			partial, err := mediaapp.NewRoutedTransferWorker(workerRepo, objects, t.TempDir()).Execute(fixtureCopyContext(t), transferExecution(f.job))

			if err != nil || partial.Status != "partial_failed" || partial.NeedsReconciliation || partial.Items[0].Status != "succeeded" || partial.Items[1].Status != "failed" {
				t.Fatal("known missing source rendition did not retain precise partial result", partial, err)
			}
			var published int64
			if err := f.db.Raw(`SELECT count(*) FROM media.media_asset WHERE id IN (?,?)`, *partial.Items[0].TargetAssetID, *partial.Items[1].TargetAssetID).Scan(&published).Error; err != nil || published != 1 {
				t.Fatal("incomplete item was published", published, err)
			}
			if err := f.objects.Put(t.Context(), missing, bytes.NewReader(data), int64(len(data)), "image/png"); err != nil {
				t.Fatal(err)
			}
			before := transferOutputFacts(t, f.db, f.job.ID)
			firstWrites := map[domain.ObjectRef]int{}
			for _, fact := range before {
				if fact.ItemIndex == 0 {
					ref := domain.ObjectRef{ProfileID: fact.ProfileID, Bucket: fact.Bucket, Key: fact.TargetObjectKey}
					firstWrites[ref] = audit.writes[ref]
				}
			}
			var frozenBefore []byte
			if err := f.db.Raw(`SELECT frozen FROM media.transfer_item WHERE job_id=? AND item_index=1`, f.job.ID).Row().Scan(&frozenBefore); err != nil {
				t.Fatal(err)
			}
			retry, err := f.repo.ControlTransfer(t.Context(), f.actor, partial.ID, uuid.New(), partial.Revision, "retry")
			if err != nil || retry.Attempt != partial.Attempt+1 || retry.Items[1].TargetItemID != partial.Items[1].TargetItemID {
				t.Fatal("retry changed target identity", retry, err)
			}
			after := transferOutputFacts(t, f.db, f.job.ID)
			if len(before) != 6 || len(after) != 9 {
				t.Fatal("retry did not append only the failed item's complete physical set", before, after)
			}
			oldTargets := map[string]transferOutputFact{}
			for _, fact := range before {
				if fact.ItemIndex == 1 {
					oldTargets[fact.RenditionKind] = fact
				}
			}
			for _, fact := range after {
				if fact.ItemIndex == 0 && fact.OutputAttempt != 1 {
					t.Fatal("published item acquired another physical attempt", fact)
				}
				if fact.ItemIndex == 1 && fact.OutputAttempt == retry.Attempt {
					old := oldTargets[fact.RenditionKind]
					if fact.TargetObjectKey == old.TargetObjectKey || fact.TargetLocationID == old.TargetLocationID || fact.ProfileID != old.ProfileID || fact.Bucket != old.Bucket {
						t.Fatal("retry reused retired identity or changed frozen route", old, fact)
					}
				}
			}
			if workerRepo.firstLease == nil {
				t.Fatal("initial lease was not observed")
			}
			if err := f.repo.Worker.BeginTransferWrite(t.Context(), *workerRepo.firstLease, 1, ""); !errors.Is(err, domain.ErrTransferConflict) {
				t.Fatal("old execution wrote new physical attempt", err)
			}
			result, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()).Execute(t.Context(), transferExecution(retry))
			if err != nil || result.Status != "succeeded" || result.Items[0].TargetItemID != partial.Items[0].TargetItemID {
				t.Fatal("precise retry failed", result, err)
			}
			for ref, count := range firstWrites {
				if count != 1 || audit.writes[ref] != count {
					t.Fatal("retry rewrote already published item's bytes", ref, count, audit.writes[ref])
				}
			}
			var frozenAfter []byte
			if err := f.db.Raw(`SELECT frozen FROM media.transfer_item WHERE job_id=? AND item_index=1`, f.job.ID).Row().Scan(&frozenAfter); err != nil || !bytes.Equal(frozenBefore, frozenAfter) {
				t.Fatal("retry rewrote original admission", err)
			}
			if replay, err := f.repo.CreateTransfer(t.Context(), f.actor, f.input); err != nil || !reflect.DeepEqual(replay, f.job) {
				t.Fatal("retry changed permanent admission receipt", replay, err)
			}
			if _, err := f.repo.Worker.ClaimTransfer(t.Context(), transferExecution(f.job)); !errors.Is(err, domain.ErrTransferConflict) {
				t.Fatal("old attempt regained execution authority", err)
			}
		})
	}
}

func TestMediaTransferMissingRenditionRetainsDebtUntilWriteFenceDrains(t *testing.T) {
	f := newTransferRecoveryFixture(t, 2)
	var missing, targetOriginal string
	if err := f.db.Raw(`SELECT source_object_key FROM media.transfer_object WHERE job_id=? AND item_index=1 AND rendition_kind='thumb_256'`, f.job.ID).Scan(&missing).Error; err != nil || missing == "" {
		t.Fatal("source rendition fixture missing", err)
	}
	if err := f.db.Raw(`SELECT target_object_key FROM media.transfer_object WHERE job_id=? AND item_index=1 AND rendition_kind=''`, f.job.ID).Scan(&targetOriginal).Error; err != nil || targetOriginal == "" {
		t.Fatal("target original fixture missing", err)
	}
	if err := f.objects.Remove(t.Context(), missing); err != nil {
		t.Fatal(err)
	}
	result, err := mediaapp.NewRoutedTransferWorker(f.repo.Worker, mediaFixtureCopyObjects(t, f.db), t.TempDir()).Execute(fixtureCopyContext(t), transferExecution(f.job))
	if err != nil || result.Status != domain.TransferStatusNeedsReconciliation || !result.NeedsReconciliation || result.Items[0].Status != domain.TransferItemStatusSucceeded || result.Items[1].Status != domain.TransferItemStatusNeedsReconciliation {
		t.Fatal("live original write fence lost precise rendition failure debt", result, err)
	}
	var published int64
	if err := f.db.Raw(`SELECT count(*) FROM media.media_asset WHERE id IN (?,?)`, *result.Items[0].TargetAssetID, *result.Items[1].TargetAssetID).Scan(&published).Error; err != nil || published != 1 {
		t.Fatal("incomplete rendition set was published", published, err)
	}
	if exists, err := f.objects.Exists(t.Context(), targetOriginal); err != nil || !exists {
		t.Fatal("live write fence allowed premature physical cleanup", exists, err)
	}
	if _, err := f.repo.ControlTransfer(t.Context(), f.actor, result.ID, uuid.New(), result.Revision, "retry"); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("unreconciled physical debt accepted retry", err)
	}
}
