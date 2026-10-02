package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestLibraryNewAudioBindingUsesOnlyActualOriginalProof(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	file := filepath.Join(t.TempDir(), "voice.wav")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "pcm_s16le", file)
	if err := cmd.Run(); err != nil {
		t.Fatal("actual sound fixture", err)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	upload := documentUpload(t, db, objects, actor, project, "voice.wav", body)
	var fact mediaapp.ReferenceFact
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		fact, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, upload.Asset.ID, domain.KindAudio)
		return err
	}); err != nil || fact.Kind != domain.KindAudio || fact.RenditionID != nil || fact.RenditionSHA256 != nil || fact.AssetID != upload.Asset.ID {
		t.Fatal("actual audio proof invented an image rendition", fact, err)
	}
	sha := sha256.Sum256(body)
	if fact.SHA256 != hex.EncodeToString(sha[:]) || fact.ByteSize != int64(len(body)) {
		t.Fatal("audio proof does not describe actual source", fact)
	}
	rends, err := pgmedia.NewStore(db).FindRenditions(t.Context(), actor, project, upload.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
}

func TestLibraryNewBindingActualOriginalAndDistinctRenditionProof(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	upload := documentUpload(t, db, objects, actor, project, "binding.png", uploadPNG(t))
	store := pgmedia.NewStore(db)
	asset, err := store.FindAsset(t.Context(), actor, project, upload.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, project, asset.ID)
	if err != nil || len(rends) != 2 {
		t.Fatal(err)
	}
	for _, r := range rends {
		t.Cleanup(func() { _ = objects.Remove(context.Background(), r.ObjectKey) })
	}
	var fact mediaapp.ReferenceFact
	query := func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			var err error
			fact, err = mediaapp.NewReferenceFactQuery(pgmedia.NewLibraryStore(tx, libraryTestAccess, time.Now), objects).Reference(t.Context(), actor, project, asset.ID, domain.KindImage)
			return err
		})
	}
	if err := query(); err != nil || fact.AssetID != asset.ID || fact.SHA256 != *asset.SHA256 || fact.ByteSize != asset.ByteSize || fact.RenditionID == nil || fact.RenditionSHA256 == nil {
		t.Fatal("actual formal proof", fact, err)
	}
	var rendition domain.Rendition
	for _, r := range rends {
		if r.ID == *fact.RenditionID {
			rendition = r
		}
	}
	reader, err := objects.Get(t.Context(), rendition.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	digest := sha256.Sum256(body)
	if *fact.RenditionSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("rendition SHA was replaced by original SHA", fact)
	}
	// Same-size garbage has an actual hash but is not a verified image rendition.
	if err := objects.Remove(t.Context(), rendition.ObjectKey); err != nil {
		t.Fatal(err)
	}
	garbage := bytes.Repeat([]byte{'x'}, len(body))
	badHash := sha256.Sum256(garbage)
	if err := objects.PutIfAbsent(t.Context(), rendition.ObjectKey, bytes.NewReader(garbage), int64(len(garbage)), "image/png", hex.EncodeToString(badHash[:])); err != nil {
		t.Fatal(err)
	}
	if err := query(); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("non-image bytes promoted to verified rendition", err)
	}
	if err := objects.Remove(t.Context(), rendition.ObjectKey); err != nil {
		t.Fatal(err)
	}
	if err := objects.PutIfAbsent(t.Context(), rendition.ObjectKey, bytes.NewReader(body), int64(len(body)), "image/png", hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: asset.ID}}}); err != nil {
		t.Fatal(err)
	}
	if err := query(); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("recycled catalog formed a new entity binding", err)
	}
	// Catalog hiding does not revoke an existing business owner's historical read.
	if _, err := mediaapp.NewAssetQuery(store, nil).Reference(t.Context(), actor, project, asset.ID); err != nil {
		t.Fatal("new binding rule silently changed historical eligibility", err)
	}
}
