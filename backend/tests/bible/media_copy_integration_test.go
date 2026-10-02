package bible_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	billingpg "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func bibleMediaBinding(b app.ProjectCopyBinding) mediaapp.ProjectCopyBinding {
	return mediaapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}
}

type bibleCopyMedia struct {
	query    *mediaapp.ProjectCopyReferenceQuery
	snapshot mediaapp.ProjectCopySnapshot
}

func (r bibleCopyMedia) FreezeReferences(ctx context.Context, actor identityapp.Principal, b app.ProjectCopyBinding, facts []domain.MediaFact, assets map[uuid.UUID]uuid.UUID) ([]app.ReferenceMapping, error) {
	input := make([]mediaapp.ReferenceFact, len(facts))
	for i, f := range facts {
		input[i] = referenceFact(f)
	}
	rows, err := r.query.FreezeReferences(ctx, actor, bibleMediaBinding(b), r.snapshot, input)
	if err != nil {
		return nil, err
	}
	out := make([]app.ReferenceMapping, len(rows))
	for i, row := range rows {
		if assets[row.Source.AssetID] != row.Target.AssetID {
			return nil, app.ErrConflict
		}
		out[i] = app.ReferenceMapping{Source: mediaFact(row.Source), Target: mediaFact(row.Target)}
	}
	return out, nil
}

type bibleTransferredReferences struct {
	db        *gorm.DB
	objects   *objectstorage.Client
	authority workspaceapp.ProjectCopyAuthority
	snapshot  mediaapp.ProjectCopySnapshot
}

func (r bibleTransferredReferences) VerifyTransferred(ctx context.Context, actor identityapp.Principal, b app.ProjectCopyBinding, rows []app.ReferenceMapping) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := (bibleCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, r.authority)}).Authorize(ctx, actor, b, true); err != nil {
			return err
		}
		input := make([]mediaapp.ProjectCopyReferenceMapping, len(rows))
		for i, row := range rows {
			input[i] = mediaapp.ProjectCopyReferenceMapping{Source: referenceFact(row.Source), Target: referenceFact(row.Target)}
		}
		return mediaapp.NewProjectCopyReferenceQuery(mediapg.NewProjectCopyStore(tx), r.objects).VerifyTransferred(ctx, actor, bibleMediaBinding(b), r.snapshot, input)
	})
}

func bibleCopyMediaStore(db *gorm.DB, authority workspaceapp.ProjectCopyAuthority, media mediaapp.ProjectCopySnapshot) *pg.ProjectCopyStore {
	return pg.NewProjectCopyStore(db, func(tx *gorm.DB) app.ProjectCopyAccess {
		return bibleCopyAccess{workspacepg.NewProjectCopyAccessStore(tx, authority)}
	}, func(tx *gorm.DB) app.ProjectCopyMedia {
		return bibleCopyMedia{mediaapp.NewProjectCopyReferenceQuery(mediapg.NewProjectCopyStore(tx), nil), media}
	}, nil)
}

func bibleCoordinatorObjects(db *gorm.DB) *workspacepg.ProjectCopyStore {
	return workspacepg.NewProjectCopyStore(db, nil, func(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
		return workspaceapp.ProjectCopyOwners{Media: mediapg.NewProjectCopyStore(tx), Canvas: canvaspg.NewProjectCopyStore(tx, nil, nil), Budget: billingpg.NewStore(tx)}
	})
}

type bibleRemoveFault struct {
	mediaapp.ProjectCopyObjects
	key string
}

func (f bibleRemoveFault) Remove(ctx context.Context, key string) error {
	if key == f.key {
		return errors.New("injected private removal failure")
	}
	return f.ProjectCopyObjects.Remove(ctx, key)
}

// This fixture uses real owning ports and a claimed workspace fence. Root's
// complete admission/publication integration remains a separate acceptance gate.
func TestBibleCopyPGActualImageAudioRetainedHistoryAndMissingBytesFence(t *testing.T) {
	db, owner := bibleTestDB(t)
	objects := bibleObjects(t)
	actor, source := bibleActorProject(t, owner)
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	imageID := bibleUploadedMedia(t, db, objects, actor, source, "historical.png", picture.Bytes())
	audioPath := filepath.Join(t.TempDir(), "historical.wav")
	if err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=660:duration=1", "-c:a", "pcm_s16le", audioPath).Run(); err != nil {
		t.Fatal(err)
	}
	audioBytes, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	audioID := bibleUploadedMedia(t, db, objects, actor, source, "historical.wav", audioBytes)
	service := app.NewService(pg.NewStore(db, pg.Factories{
		Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) },
		Media: func(tx *gorm.DB) app.MediaReferences {
			return actualBibleMedia{mediaapp.NewReferenceFactQuery(mediapg.NewLibraryStore(tx, bibleLibraryFactory, time.Now), objects)}
		},
	}), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(source))
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.Find(t.Context(), actor, source, domain.KindCharacter, first.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	look := detail.Current.Character.Looks[0].ID
	second, err := service.Change(t.Context(), actor, app.Command{ProjectID: source, Kind: domain.KindCharacter, Action: "references", EntryID: first.EntryID, ExpectedRevision: first.Revision, LookID: &look, References: []app.ReferenceInput{{Role: domain.RolePrimary, AssetID: imageID}}, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	third, err := service.Change(t.Context(), actor, app.Command{ProjectID: source, Kind: domain.KindCharacter, Action: "voice_bind", EntryID: first.EntryID, ExpectedRevision: second.Revision, Voice: &app.VoiceInput{Kind: domain.VoiceSample, Instructions: "旧声样与素材必须保留", Sample: &app.SampleInput{Name: "历史声样", AssetID: audioID}}, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: source, Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: third.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	// The owning soft-delete retains bytes. Version fact rev 1 is not rewritten;
	// media's explicit retained-history proof relates it to current rev 2.
	for _, id := range []uuid.UUID{imageID, audioID} {
		asset, err := mediapg.NewStore(db).FindAsset(t.Context(), actor, source, id)
		if err != nil {
			t.Fatal(err)
		}
		deleted, err := asset.Delete(time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=?`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	b, authority := bibleCopyFixture(t, owner, actor, source)
	admission := authority
	admission.Phase, admission.WorkerID = "freeze", uuid.Nil
	var media mediaapp.ProjectCopySnapshot
	var snapshot app.ProjectCopySnapshot
	if err := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		facts, err := bibleCopyStore(tx, admission).ReferencedMediaFacts(t.Context(), actor, b)
		if err != nil {
			return err
		}
		input := make([]mediaapp.ReferenceFact, len(facts))
		for i, f := range facts {
			input[i] = referenceFact(f)
		}
		media, err = mediapg.NewProjectCopyStore(tx).FreezeWithReferenceFacts(t.Context(), actor, bibleMediaBinding(b), time.Now(), nil, input)
		if err != nil {
			return err
		}
		snapshot, err = bibleCopyMediaStore(tx, admission, media).Freeze(t.Context(), actor, b, media.AssetMapping, time.Now())
		return err
	}); err != nil {
		t.Fatal("SQL-only all historical media plan", err)
	}
	if media.Assets != 2 || snapshot.Counts.References != 2 || snapshot.Counts.Voices != 1 {
		t.Fatal("historical media omitted", media, snapshot.Counts)
	}
	manifest := workspacedomain.ProjectCopyManifest{WorkspaceSHA256: strings.Repeat("a", 64), CanvasSnapshotID: uuid.New(), CanvasSHA256: strings.Repeat("a", 64), MediaSnapshotID: media.ID, MediaSHA256: media.ManifestSHA256, Assets: media.Assets, Renditions: media.Renditions, Script: &workspacedomain.ProjectCopyScriptSnapshot{ID: uuid.New(), ManifestSHA256: strings.Repeat("a", 64), ContentSHA256: strings.Repeat("a", 64)}}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture transition only installs the actual frozen media facts. It
	// does not stand in for production admission or whole project publication.
	if err := owner.Exec(`UPDATE workspace.project_copy_job SET stage='media',media_receipt=NULL,manifest=?::jsonb WHERE id=?`, string(body), b.JobID).Error; err != nil {
		t.Fatal(err)
	}
	coordinator := bibleCoordinatorObjects(db)
	repo := workspacepg.NewProjectCopyMediaObjects(coordinator, b.JobID, authority.WorkerID, false)
	stale := workspacepg.NewProjectCopyMediaObjects(coordinator, b.JobID, uuid.New(), false)
	if err := mediaapp.NewProjectCopyTransfer(stale, mediaobjects.NewProjectCopyObjects(objects), t.TempDir()).Transfer(t.Context(), actor, bibleMediaBinding(b), media); !errors.Is(err, workspacedomain.ErrProjectCopyWorkerConflict) {
		t.Fatal("stale media worker accepted", err)
	}
	if err := mediaapp.NewProjectCopyTransfer(repo, mediaobjects.NewProjectCopyObjects(objects), t.TempDir()).Transfer(t.Context(), actor, bibleMediaBinding(b), media); err != nil {
		t.Fatal("actual fenced private transfer", err)
	}
	intents, err := repo.Objects(t.Context(), actor, bibleMediaBinding(b), media)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, object := range intents {
			if err := objects.Remove(ctx, object.TargetObjectKey); err != nil {
				t.Error(err)
			}
		}
	})
	var receipt mediaapp.ProjectCopyReceipt
	if err := coordinator.UseAttempt(t.Context(), actor, b.JobID, authority.WorkerID, "media", func(owners workspaceapp.ProjectCopyOwners, _ workspacedomain.ProjectCopyJob) error {
		var err error
		receipt, err = owners.Media.Register(t.Context(), actor, bibleMediaBinding(b), media)
		return err
	}); err != nil {
		t.Fatal("actual media registration", err)
	}
	receiptBody, err := json.Marshal(workspacedomain.ProjectCopyReceipt{ManifestSHA256: receipt.ManifestSHA256, ContentSHA256: receipt.ContentSHA256, PrimaryCount: receipt.Assets, SecondaryCount: receipt.Renditions})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE workspace.project_copy_job SET stage='script',media_receipt=?::jsonb WHERE id=?`, string(receiptBody), b.JobID).Error; err != nil {
		t.Fatal(err)
	}
	copyStore := bibleCopyMediaStore(db, authority, media)
	consumer := app.NewProjectCopy(copyStore, bibleTransferredReferences{db, objects, authority, media})
	object := intents[0]
	reader, err := objects.Get(t.Context(), object.TargetObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	original, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	if err := objects.Remove(t.Context(), object.TargetObjectKey); err != nil {
		t.Fatal(err)
	}
	err = consumer.Transfer(t.Context(), actor, b, snapshot)
	var unknown *app.ProjectCopyTransferError
	if !errors.As(err, &unknown) || !unknown.NeedsReconciliation {
		t.Fatal("missing private bytes released fence", err)
	}
	var n int64
	if err := owner.Table("bible.copy_transfer_receipt").Where("snapshot_id=?", snapshot.ID).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("false verified Bible receipt", n, err)
	}
	if object.SHA256 == nil {
		t.Fatal("missing actual object digest")
	}
	if err := objects.PutIfAbsent(t.Context(), object.TargetObjectKey, bytes.NewReader(original), int64(len(original)), object.ContentType, *object.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Transfer(t.Context(), actor, b, snapshot); err != nil {
		t.Fatal("exact original object recovery", err)
	}
	register := authority
	register.Phase = "register"
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := bibleCopyMediaStore(tx, register, media).Register(t.Context(), actor, b, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m, err := copyStore.Manifest(t.Context(), actor, b, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range m.References {
		if row.Source.Revision != 1 || row.Target.Revision != 1 || row.Source.AssetID == row.Target.AssetID {
			t.Fatal("historical revision/independent identity lost", row)
		}
		if row.Source.Kind == "image" && (row.Source.RenditionID == row.Target.RenditionID || row.Source.RenditionSHA256 != row.Target.RenditionSHA256) {
			t.Fatal("independent rendition not proven", row)
		}
	}
	// Only the exact current cancellation attempt can seal hidden SQL history
	// and remove the independent target objects. Source objects remain intact.
	if err := owner.Exec(`UPDATE workspace.project_copy_job SET status='cancel_requested',stage='cleanup',cancellation_requested=true WHERE id=?`, b.JobID).Error; err != nil {
		t.Fatal(err)
	}
	cleanup := authority
	cleanup.Phase = "cleanup"
	if err := db.Transaction(func(tx *gorm.DB) error {
		return bibleCopyMediaStore(tx, cleanup, media).Cleanup(t.Context(), actor, b, snapshot)
	}); err != nil {
		t.Fatal("seal unpublished immutable history", err)
	}
	cleanupRepo := workspacepg.NewProjectCopyMediaObjects(coordinator, b.JobID, authority.WorkerID, true)
	fault := bibleRemoveFault{ProjectCopyObjects: mediaobjects.NewProjectCopyObjects(objects), key: intents[0].TargetObjectKey}
	err = mediaapp.NewProjectCopyTransfer(cleanupRepo, fault, t.TempDir()).Cleanup(t.Context(), actor, bibleMediaBinding(b), media)
	var removal *mediaapp.ProjectCopyTransferError
	if !errors.As(err, &removal) || !removal.NeedsReconciliation || removal.Code != "object_remove_unknown" {
		t.Fatal("unknown removal released cleanup fence", err)
	}
	if err := mediaapp.NewProjectCopyTransfer(cleanupRepo, mediaobjects.NewProjectCopyObjects(objects), t.TempDir()).Cleanup(t.Context(), actor, bibleMediaBinding(b), media); err != nil {
		t.Fatal("same frozen object cleanup recovery", err)
	}
	if err := coordinator.UseAttempt(t.Context(), actor, b.JobID, authority.WorkerID, "cleanup", func(owners workspaceapp.ProjectCopyOwners, _ workspacedomain.ProjectCopyJob) error {
		return owners.Media.FinishCleanup(t.Context(), actor, bibleMediaBinding(b), media)
	}); err != nil {
		t.Fatal(err)
	}
	for _, object := range intents {
		if exists, err := objects.Exists(t.Context(), object.TargetObjectKey); err != nil || exists {
			t.Fatal("target bytes remain after actual confirmed cleanup", exists, err)
		}
		if exists, err := objects.Exists(t.Context(), object.SourceObjectKey); err != nil || !exists {
			t.Fatal("source history was destroyed by copy cancellation", exists, err)
		}
	}
	if err := owner.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, b.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return bibleCopyMediaStore(tx, cleanup, media).Cleanup(t.Context(), actor, b, snapshot)
	}); !errors.Is(err, workspacedomain.ErrProjectCopyStateConflict) {
		t.Fatal("cleanup authorized an active published target", err)
	}
}
