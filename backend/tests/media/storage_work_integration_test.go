package media_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestStorageWorkIntentPersistsAcrossSwitchAndWorkerRestart(t *testing.T) {
	db := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	current := activeTestStorageRoute(t, routes)
	rev := "work-old-" + uuid.NewString()
	if _, err := routes.Activate(t.Context(), current.Revision, rev, "minio-main"); err != nil {
		t.Fatal(err)
	}
	owner := mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: uuid.New(), WorkID: uuid.New()}
	first, err := routes.PinStorageWork(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routes.Activate(t.Context(), rev, "work-new-"+uuid.NewString(), "cos-main"); err != nil {
		t.Fatal(err)
	}
	restarted := pgmedia.NewStorageRoutes(db)
	replay, err := restarted.PinStorageWork(t.Context(), owner)
	if err != nil || replay != first {
		t.Fatal("restarted worker rebound original intent", err)
	}
	next, err := restarted.PinStorageWork(t.Context(), mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: owner.OwnerID, WorkID: uuid.New()})
	if err != nil || next.ProfileID != "cos-main" {
		t.Fatal("new work ignored activated route", err)
	}
	location, err := restarted.ReserveWorkLocation(t.Context(), first.ID, "projects/"+owner.WorkID.String()+"/image/result.png")
	if err != nil || location.Ref.ProfileID != "minio-main" {
		t.Fatal("late object used current profile", err)
	}
	info := mediaapp.ObjectInfo{Size: 3, ContentType: "image/png", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := routes.ConfirmWorkLocation(t.Context(), first.ID, location, info); err != nil {
		t.Fatal(err)
	}
	if err := routes.AcceptStorageWork(t.Context(), first.ID, []uuid.UUID{location.ID}); err != nil {
		t.Fatal(err)
	}
	remove, err := routes.BeginWorkRemoval(t.Context(), first.ID, location)
	if err != nil || remove {
		t.Fatal("accepted work admitted to compensation", err)
	}
	if _, err := routes.ReserveWorkLocation(t.Context(), first.ID, "projects/old/image/extra.png"); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatal("accepted intent gained new output", err)
	}
}
