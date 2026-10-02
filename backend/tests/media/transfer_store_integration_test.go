package media_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type transferTestWorkGuard struct{ media *pgmedia.Store }

func (g transferTestWorkGuard) HasInflightProjectWork(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (bool, error) {
	return g.media.HasInflightWork(ctx, actor, id)
}
func transferTestGuards(tx *gorm.DB) mediaapp.LibraryWorkGuards {
	return transferTestWorkGuard{pgmedia.NewStore(tx)}
}

func TestMediaTransferActualAdmissionPendingVisibilityAndPermanentCurrentScopeReplay(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	base := pgmedia.NewStore(db)
	uploadService := mediaapp.NewScopedUploadService(base, base, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = uuid.Nil
	upload, err := uploadService.UploadPersonal(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE id=? UNION ALL SELECT object_key FROM media.rendition WHERE media_asset_id=?`, upload.Asset.ID, upload.Asset.ID).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			_ = objects.Remove(ctx, key)
		}
	})
	request := mediaapp.TransferInput{Source: domain.LibraryScope{Kind: domain.LibraryPersonal}, Target: domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID}}, ExpectedSourceRevision: 1, ExpectedProjectRevision: 1, Key: uuid.New()}
	repo := pgmedia.NewTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
	job, err := repo.CreateTransfer(t.Context(), actor, request)
	if err != nil || job.Status != "queued" || len(job.Items) != 1 || job.Items[0].TargetAssetID == nil || *job.Items[0].TargetAssetID == upload.Asset.ID {
		t.Fatal("durable scoped admission", job, err)
	}
	page, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ListLibrary(t.Context(), actor, request.Target, libraryQuery())
	if err != nil || page.Total != 0 {
		t.Fatal("unpublished transfer entered ordinary catalog", page, err)
	}
	if _, err := base.FindAsset(t.Context(), actor, project, *job.Items[0].TargetAssetID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("pending target became formal reference", err)
	}
	replay, err := repo.CreateTransfer(t.Context(), actor, request)
	if err != nil || !reflect.DeepEqual(job, replay) {
		t.Fatal("permanent admission replay changed IDs", replay, err)
	}
	changed := request
	changed.ExpectedSourceRevision = 2
	if _, err := repo.CreateTransfer(t.Context(), actor, changed); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("same key different immutable input accepted", err)
	}
	other, _ := mediaStoreProject(t, db)
	if _, err := repo.GetTransfer(t.Context(), other, job.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("personal source transfer leaked to another actor", err)
	}
}
