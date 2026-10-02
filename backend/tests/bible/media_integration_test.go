package bible_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

type bibleLibraryAccess struct {
	owner *workspacepg.ProjectContentAccessStore
}

func (a bibleLibraryAccess) Authorize(ctx context.Context, actor identityapp.Principal, p uuid.UUID, write bool) (mediaapp.LibraryProjectFacts, error) {
	f, err := a.owner.Authorize(ctx, actor, p, write)
	return mediaapp.LibraryProjectFacts{ProjectID: f.ProjectID, OrgID: f.OrgID, Revision: f.Revision}, err
}
func (a bibleLibraryAccess) TouchContent(ctx context.Context, actor identityapp.Principal, p uuid.UUID, r int64) (int64, error) {
	return a.owner.TouchContent(ctx, actor, p, r)
}
func bibleLibraryFactory(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return bibleLibraryAccess{workspacepg.NewProjectContentAccessStore(tx)}
}

func mediaFact(f mediaapp.ReferenceFact) domain.MediaFact {
	result := domain.MediaFact{AssetID: f.AssetID, Revision: f.Revision, Kind: string(f.Kind), SHA256: f.SHA256, ByteSize: f.ByteSize}
	if f.RenditionID != nil {
		result.RenditionID = *f.RenditionID
	}
	if f.RenditionSHA256 != nil {
		result.RenditionSHA256 = *f.RenditionSHA256
	}
	return result
}
func referenceFact(f domain.MediaFact) mediaapp.ReferenceFact {
	result := mediaapp.ReferenceFact{AssetID: f.AssetID, Revision: f.Revision, Kind: mediadomain.Kind(f.Kind), SHA256: f.SHA256, ByteSize: f.ByteSize}
	if f.Kind == "image" {
		id, sha := f.RenditionID, f.RenditionSHA256
		result.RenditionID = &id
		result.RenditionSHA256 = &sha
	}
	return result
}

type actualBibleMedia struct{ owner *mediaapp.ReferenceFactQuery }

func (a actualBibleMedia) Reference(ctx context.Context, actor identityapp.Principal, p, id uuid.UUID, kind string) (domain.MediaFact, error) {
	f, err := a.owner.Reference(ctx, actor, p, id, mediadomain.Kind(kind))
	return mediaFact(f), err
}
func (a actualBibleMedia) Verify(ctx context.Context, actor identityapp.Principal, p uuid.UUID, f domain.MediaFact) error {
	return a.owner.VerifyFrozenReference(ctx, actor, p, referenceFact(f))
}

func bibleUploadedMedia(t *testing.T, db *gorm.DB, objects *objectstorage.Client, actor identityapp.Principal, project uuid.UUID, name string, body []byte) uuid.UUID {
	t.Helper()
	input, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(body), name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := input.Close(); err != nil {
			t.Error(err)
		}
	})
	uploader := mediaapp.NewUploadService(mediapg.NewStore(db), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	result, err := uploader.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: name}, File: input, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("actual owner-reviewed upload", err)
	}
	asset, err := mediapg.NewStore(db).FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	rends, err := mediapg.NewStore(db).FindRenditions(t.Context(), actor, project, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{asset.ObjectKey}
	for _, r := range rends {
		keys = append(keys, r.ObjectKey)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			if err := objects.Remove(ctx, key); err != nil {
				t.Error("remove synthetic uploaded object", err)
			}
		}
	})
	return asset.ID
}

func TestBiblePGActualSixImageRolesSampleAndFrozenTrashBoundaries(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	actor, project := bibleActorProject(t, owner)
	var imageBytes bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			picture.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 7), B: 97, A: 255})
		}
	}
	if err := png.Encode(&imageBytes, picture); err != nil {
		t.Fatal(err)
	}
	imageID := bibleUploadedMedia(t, db, objects, actor, project, "reference.png", imageBytes.Bytes())
	audioPath := filepath.Join(t.TempDir(), "voice.wav")
	if err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "pcm_s16le", audioPath).Run(); err != nil {
		t.Fatal("actual audio fixture", err)
	}
	audioBytes, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	audioID := bibleUploadedMedia(t, db, objects, actor, project, "voice.wav", audioBytes)
	factories := pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, Media: func(tx *gorm.DB) app.MediaReferences {
		return actualBibleMedia{mediaapp.NewReferenceFactQuery(mediapg.NewLibraryStore(tx, bibleLibraryFactory, time.Now), objects)}
	}}
	service := app.NewService(pg.NewStore(db, factories), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.Find(t.Context(), actor, project, domain.KindCharacter, first.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	look := detail.Current.Character.Looks[0].ID
	refs := make([]app.ReferenceInput, 0, 6)
	for _, role := range []domain.ImageRole{domain.RolePrimary, domain.RoleFront, domain.RoleSide, domain.RoleBack, domain.RoleTurnaround, domain.RoleExpression} {
		refs = append(refs, app.ReferenceInput{Role: role, AssetID: imageID})
	}
	second, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "references", EntryID: first.EntryID, ExpectedRevision: 1, LookID: &look, References: refs, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal("actual image reference consumer", err)
	}
	third, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "voice_bind", EntryID: first.EntryID, ExpectedRevision: 2, Voice: &app.VoiceInput{Kind: domain.VoiceSample, Instructions: "轻声说话，保留停顿。", Sample: &app.SampleInput{Name: "角色声样", AssetID: audioID}}, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal("actual audio sample consumer", err)
	}
	current, err := service.Find(t.Context(), actor, project, domain.KindCharacter, first.EntryID)
	if err != nil || len(current.Current.Character.Looks[0].References) != 6 || current.Current.Character.Voice.Sample.Media.AssetID != audioID || current.Current.Character.Voice.Kind != domain.VoiceSample {
		t.Fatal("typed roles/sample lost", err)
	}
	for _, ref := range current.Current.Character.Looks[0].References {
		if ref.Media.RenditionID == uuid.Nil || ref.Media.RenditionSHA256 == "" {
			t.Fatal("original masquerades as image rendition")
		}
	}
	old, err := service.Version(t.Context(), actor, project, domain.KindCharacter, first.EntryID, second.VersionID)
	if err != nil || old.Character.Voice != nil {
		t.Fatal("sample rewrote prior immutable version", err)
	}
	if _, err := mediapg.NewLibraryStore(db, bibleLibraryFactory, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &project}, Action: "recycle_items", Key: uuid.New(), Items: []mediaapp.LibraryItemRevision{{ID: imageID}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: third.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal("catalog trash revoked frozen historical evidence", err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "references", EntryID: first.EntryID, ExpectedRevision: 4, LookID: &look, References: refs, Key: uuid.New(), RequestID: uuid.New()}); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("hidden catalog promoted into new binding", err)
	}
	var objectKey string
	if err := owner.Raw(`SELECT object_key FROM media.rendition WHERE id=?`, current.Current.Character.Looks[0].References[0].Media.RenditionID).Scan(&objectKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := objects.Remove(t.Context(), objectKey); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: 4, Key: uuid.New(), RequestID: uuid.New()}); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing private rendition published as confirmed", err)
	}
}
