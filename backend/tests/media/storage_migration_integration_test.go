package media_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestStorageNativeBindingCannotBeRepointed(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	_, project := mediaStoreProject(t, db)
	a := sourceDocumentAsset([]byte("history"))
	a.ProjectID = project
	a.ObjectKey = "projects/" + project.String() + "/document/2026/10/" + a.ID.String() + ".txt"
	original, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: "cos-first", Bucket: "storage-test-1250000000", Key: a.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: "cos-main", Bucket: "storage-test-123", Key: a.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,codec,moderation_status,revision,object_location_id) VALUES(?,?,'document','upload','ready',?,?,?,?,?,'txt','passed',1,?)`, a.ID, project, a.ObjectKey, a.FileName, a.MimeType, a.ByteSize, *a.SHA256, original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET object_location_id=? WHERE id=?`, replacement.ID, a.ID).Error; err == nil {
		t.Fatal("a historical file was rebound to another platform")
	}
}

func TestStorageNativeWorkVerifiesFactsAndFencesConcurrentChanges(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, db, "native-"+uuid.NewString(), "cos-first")); err != nil {
		t.Fatal(err)
	}
	owner := mediaapp.StorageWorkOwner{Kind: mediaapp.StorageWorkKindOperation, OwnerID: uuid.New(), WorkID: uuid.New()}
	work, err := routes.PinStorageWork(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReserveWorkLocation(t.Context(), work.ID, "projects/"+owner.OwnerID.String()+"/image/exact.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := routes.ConfirmWorkLocation(t.Context(), work.ID, location, mediaapp.ObjectInfo{Size: 1, ContentType: "image/png", SHA256: "invalid"}); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("invalid digest accepted", err)
	}
	info := mediaapp.ObjectInfo{Size: 3, ContentType: "image/png", SHA256: strings.Repeat("a", 64)}
	if err := routes.ConfirmWorkLocation(t.Context(), work.ID, location, info); err != nil {
		t.Fatal(err)
	}
	bad := info
	bad.Size++
	if err := routes.ConfirmWorkLocation(t.Context(), work.ID, location, bad); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatal("frozen metadata replaced", err)
	}
	if err := routes.AcceptStorageWork(t.Context(), work.ID, []uuid.UUID{uuid.New()}); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatal("foreign set accepted", err)
	}
	if err := routes.AcceptStorageWork(t.Context(), work.ID, []uuid.UUID{location.ID}); err != nil {
		t.Fatal(err)
	}
	if err := routes.AcceptStorageWork(t.Context(), work.ID, []uuid.UUID{location.ID}); err != nil {
		t.Fatal("lost accept reply not idempotent", err)
	}
	if err := routes.ConfirmWorkLocation(t.Context(), work.ID, location, info); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatal("closed work changed", err)
	}
	saved, err := routes.ReadLocation(t.Context(), location.ID)
	if err != nil || saved.State != domain.ObjectLocationAccepted || saved.Ref != location.Ref {
		t.Fatal("accepted location changed", err)
	}
}
