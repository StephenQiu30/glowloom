package media_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestLibraryFrozenAudioReferenceKeepsOriginalProofWithoutImageRendition(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	file := filepath.Join(t.TempDir(), "frozen.wav")
	if err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=660:duration=1", "-c:a", "pcm_s16le", file).Run(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	upload := documentUpload(t, db, objects, actor, project, "frozen.wav", body)
	var fact mediaapp.ReferenceFact
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		fact, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, upload.Asset.ID, domain.KindAudio)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rends, err := pgmedia.NewStore(db).FindRenditions(t.Context(), actor, project, upload.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	if _, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "remove_items", Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).VerifyFrozenReference(t.Context(), actor, project, fact)
	}); err != nil || fact.RenditionID != nil || fact.RenditionSHA256 != nil {
		t.Fatal("existing actual audio acquired an image proof", fact, err)
	}
}

func TestLibraryFrozenImageReferenceSurvivesCatalogTrashAndRemoval(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	upload := documentUpload(t, db, objects, actor, project, "frozen-reference.png", uploadPNG(t))
	var frozen mediaapp.ReferenceFact
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		frozen, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, upload.Asset.ID, domain.KindImage)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rends, err := pgmedia.NewStore(db).FindRenditions(t.Context(), actor, project, upload.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
	verify := func(principal identityapp.Principal, pid uuid.UUID, fact mediaapp.ReferenceFact) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).VerifyFrozenReference(t.Context(), principal, pid, fact)
		})
	}
	if err := verify(actor, project, frozen); err != nil {
		t.Fatal("fresh immutable proof", err)
	}
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	for i, action := range []string{"recycle_items", "restore_items", "remove_items"} {
		if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: action, ExpectedRevision: int64(i), Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID, Revision: int64(i)}}}); err != nil {
			t.Fatal(action, err)
		}
		if err := verify(actor, project, frozen); err != nil {
			t.Fatal("catalog visibility invalidated unchanged owning evidence", action, err)
		}
		if action == "restore_items" {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, err := mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, frozen.AssetID, domain.KindImage)
			return err
		}); !errors.Is(err, mediaapp.ErrNotFound) {
			t.Fatal("hidden catalog permitted a new binding", action, err)
		}
	}
	for _, change := range []func(*mediaapp.ReferenceFact){
		func(f *mediaapp.ReferenceFact) { f.Revision++ },
		func(f *mediaapp.ReferenceFact) { f.ByteSize++ },
		func(f *mediaapp.ReferenceFact) {
			f.SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		func(f *mediaapp.ReferenceFact) { id := uuid.New(); f.RenditionID = &id },
		func(f *mediaapp.ReferenceFact) {
			sha := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			f.RenditionSHA256 = &sha
		},
	} {
		changed := frozen
		change(&changed)
		if err := verify(actor, project, changed); !errors.Is(err, mediaapp.ErrUnavailable) {
			t.Fatal("replacement evidence accepted", err)
		}
	}
	foreign := actor
	foreign.ID = uuid.New()
	if err := verify(foreign, project, frozen); err == nil {
		t.Fatal("foreign actor verified a private frozen reference")
	}
	_, foreignProject := mediaStoreProject(t, db)
	if err := verify(actor, foreignProject, frozen); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("cross-project immutable proof accepted", err)
	}
	if err := objects.Remove(t.Context(), rends[0].ObjectKey); err != nil {
		t.Fatal(err)
	}
	if err := verify(actor, project, frozen); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("physically missing frozen rendition accepted", err)
	}
}

func TestLibraryFrozenReferenceRejectsMissingOriginalAndUnprovedSoftDeletion(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	upload := documentUpload(t, db, objects, actor, project, "frozen-original.png", uploadPNG(t))
	var frozen mediaapp.ReferenceFact
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		frozen, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, upload.Asset.ID, domain.KindImage)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	store := pgmedia.NewStore(db)
	asset, err := store.FindAsset(t.Context(), actor, project, frozen.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, project, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
	verify := func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			return mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).VerifyFrozenReference(t.Context(), actor, project, frozen)
		})
	}
	deleted, err := asset.Delete(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=?`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := verify(); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("changed revision was inferred to be the old immutable proof", err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=false,delete_time=NULL,purge_after=NULL,revision=?,update_time=? WHERE id=?`, asset.Revision, asset.UpdateTime, asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := objects.Remove(t.Context(), asset.ObjectKey); err != nil {
		t.Fatal(err)
	}
	if err := verify(); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("physically missing original accepted", err)
	}
}
