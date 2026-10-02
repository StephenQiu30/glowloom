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

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

type transferRecoveryFixture struct {
	db      *gorm.DB
	objects *objectstorage.Client
	repo    *pgmedia.TransferStore
	actor   identityapp.Principal
	input   mediaapp.TransferInput
	job     domain.TransferJob
}

func newTransferRecoveryFixture(t *testing.T, count int) transferRecoveryFixture {
	t.Helper()
	db, objects := libraryRuntimeDB(t), glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	base := pgmedia.NewStore(db)
	uploads := mediaapp.NewScopedUploadService(base, base, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
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
		result, err := uploads.UploadPersonal(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		input.Items = append(input.Items, mediaapp.LibraryItemRevision{ID: result.Asset.ID})
	}
	repo := pgmedia.NewTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
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
	mediaapp.ProjectCopyObjects
	writes, failAt int
}

func (o *transferUnknownWrite) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, digest string) error {
	err := o.ProjectCopyObjects.PutIfAbsent(ctx, key, reader, size, mime, digest)
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
	objects := &transferUnknownWrite{ProjectCopyObjects: copyobjects.NewProjectCopyObjects(f.objects), failAt: 1}
	unknown, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
	if err != nil || unknown.Status != "needs_reconciliation" || !unknown.NeedsReconciliation || unknown.ExecutionUnconfirmed || unknown.Items[0].Status != "needs_reconciliation" {
		t.Fatal("unknown physical write was treated as terminal", unknown, err)
	}
	if _, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, uuid.New(), unknown.Revision, "retry"); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("unsafe unknown write was automatically retried", err)
	}
	key := uuid.New()
	reconcile, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "reconcile")
	if err != nil || reconcile.Attempt != 2 || reconcile.Items[0].TargetItemID != unknown.Items[0].TargetItemID {
		t.Fatal("reconciliation changed independent target identity", reconcile, err)
	}
	result, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(reconcile))
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
			objects := &transferUnknownWrite{ProjectCopyObjects: copyobjects.NewProjectCopyObjects(f.objects), failAt: (count-1)*3 + 1}
			unknown, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
			if err != nil || unknown.Status != "needs_reconciliation" {
				t.Fatal(unknown, err)
			}
			key := uuid.New()
			requested, err := f.repo.ControlTransfer(t.Context(), f.actor, unknown.ID, key, unknown.Revision, "cancel")
			if err != nil || !requested.CancellationRequested || requested.Status != "cancel_requested" {
				t.Fatal("cancel intent was not retained independently", requested, err)
			}
			result, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(requested))
			wantStatus := map[int]string{1: "cancelled", 2: "partial_failed"}[count]
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

func TestMediaTransferPartialFailureCleansOnlyFailedItemAndRetriesSameIdentity(t *testing.T) {
	f := newTransferRecoveryFixture(t, 2)
	var missing string
	if err := f.db.Raw(`SELECT source_object_key FROM media.transfer_object WHERE job_id=? AND item_index=1 AND rendition_kind='thumb_256'`, f.job.ID).Scan(&missing).Error; err != nil || missing == "" {
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
	objects := copyobjects.NewProjectCopyObjects(f.objects)
	partial, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
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
	retry, err := f.repo.ControlTransfer(t.Context(), f.actor, partial.ID, uuid.New(), partial.Revision, "retry")
	if err != nil || retry.Attempt != 2 || retry.Items[1].TargetItemID != partial.Items[1].TargetItemID {
		t.Fatal("retry changed target identity", retry, err)
	}
	result, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(retry))
	if err != nil || result.Status != "succeeded" || result.Items[0].TargetItemID != partial.Items[0].TargetItemID {
		t.Fatal("precise retry failed", result, err)
	}
	if _, err := f.repo.ClaimTransfer(t.Context(), transferExecution(f.job)); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("old attempt regained execution authority", err)
	}
}
