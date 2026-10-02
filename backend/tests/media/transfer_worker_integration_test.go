package media_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestMediaTransferActualPrivateOriginalAndAllRenditionsBecomeIndependentProjectAssets(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	base := pgmedia.NewStore(db)
	uploads := mediaapp.NewScopedUploadService(base, base, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	in := uploadInput(t)
	in.Actor, in.Request.ProjectID = actor, uuid.Nil
	upload, err := uploads.UploadPersonal(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	var sourceKeys []string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION ALL SELECT object_key FROM media.rendition WHERE media_asset_id=?`, upload.Asset.ID, upload.Asset.ID).Scan(&sourceKeys).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range sourceKeys {
			_ = objects.Remove(ctx, key)
		}
	})
	repo := pgmedia.NewTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
	job, err := repo.CreateTransfer(t.Context(), actor, mediaapp.TransferInput{Source: domain.LibraryScope{Kind: domain.LibraryPersonal}, Target: domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID}}, ExpectedSourceRevision: 1, ExpectedProjectRevision: 1, Key: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	work := mediaapp.TransferWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()}
	result, err := mediaapp.NewTransferWorker(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir()).Execute(t.Context(), work)
	if err != nil || result.Status != "succeeded" || len(result.Items) != 1 || result.Items[0].Status != "succeeded" || result.ExecutionUnconfirmed || result.NeedsReconciliation {
		t.Fatal("actual scoped transfer", result, err)
	}
	asset, err := base.FindAsset(t.Context(), actor, project, *result.Items[0].TargetAssetID)
	if err != nil || asset.ID == upload.Asset.ID || !asset.CanReference() || asset.UploadID != nil || asset.SourceOperationID != nil {
		t.Fatal("target is an independent formal asset", asset, err)
	}
	rends, err := base.FindRenditions(t.Context(), actor, project, asset.ID)
	if err != nil || len(rends) != 2 {
		t.Fatal("complete rendition transfer", rends, err)
	}
	keys := []string{asset.ObjectKey}
	for _, r := range rends {
		keys = append(keys, r.ObjectKey)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = objects.Remove(context.Background(), key)
		}
	})
	for _, key := range keys {
		reader, err := objects.Get(t.Context(), key)
		if err != nil {
			t.Fatal("actual independent private bytes", err)
		}
		n, err := io.Copy(io.Discard, reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil || n < 1 {
			t.Fatal(err, closeErr)
		}
	}
	if err := objects.Remove(t.Context(), sourceKeys[0]); err != nil {
		t.Fatal(err)
	}
	reader, err := objects.Get(t.Context(), asset.ObjectKey)
	if err != nil {
		t.Fatal("source removal invalidated independent target", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	replay, err := mediaapp.NewTransferWorker(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir()).Execute(t.Context(), work)
	if err != nil || replay.ID != result.ID || replay.Revision != result.Revision {
		t.Fatal("completed delivery reexecuted physical ownership", replay, err)
	}
}
