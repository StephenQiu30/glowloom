package media_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func retainedToolDatabase(t *testing.T) (*gorm.DB, identityapp.Principal, uuid.UUID) {
	t.Helper()
	database := mediaStoreDB(t)
	var name string
	if err := database.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil || name != "lanverse_reference" {
		t.Fatal("isolated reference database required", err)
	}
	actor, project := mediaStoreProject(t, database)
	return database, actor, project
}

func TestMediaToolRetainedReaderProtectsFrozenSourcesAndUnpublishedDepthObjects(t *testing.T) {
	database, actor, project := retainedToolDatabase(t)
	timeline := exportTimeline(t)
	source := imageAsset(project, *timeline.Clips[0].AssetID)
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	source.SHA256 = &digest
	source.Status, source.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
	if err := pgmedia.NewStore(database).CreateAsset(t.Context(), actor, source); err != nil {
		t.Fatal(err)
	}
	// This fixture isolates owning metadata retention; it does not prove bytes.
	if err := database.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	store := toolpg.NewStore(database, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(pgmedia.NewStore(tx)) })
	job, err := store.Create(t.Context(), actor, project, uuid.New(), toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	// These are exact owning metadata fixtures; no inference or stored byte claim.
	video := tooldomain.FrozenSource{AssetID: uuid.New(), Revision: 1, Kind: "video", ObjectKey: "projects/" + project.String() + "/video/fixture/source.mp4", MIMEType: "video/mp4", ByteSize: 100, SHA256: digest}
	duration, width, height := int32(2000), int32(64), int32(64)
	video.DurationMS, video.Width, video.Height = &duration, &width, &height
	depth := tooldomain.FrozenDepth{ProfileID: tooldomain.DepthProfileID, Input: video}
	frozen, err := json.Marshal(depth)
	if err != nil {
		t.Fatal(err)
	}
	depthID := uuid.New()
	now := time.Now().UTC()
	frozenHash := sha256.Sum256(frozen)
	if err := database.Exec(`INSERT INTO mediatool.depth_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,source_asset_id,source_asset_revision,source_sha256,profile_id,frozen,frozen_sha256,status,stage,attempt,revision,process_state,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,?,1,?, ?,?::jsonb,?,'failed','awaiting_reconciliation',1,1,'ended',?,?)`, depthID, project, actor.OrgID, actor.ID, uuid.New(), uuid.New(), video.AssetID, digest, tooldomain.DepthProfileID, string(frozen), hex.EncodeToString(frozenHash[:]), now, now).Error; err != nil {
		t.Fatal(err)
	}
	depthJob := tooldomain.DepthJob{ID: depthID, ProjectID: project, Attempt: 1, CreatedAt: now}
	outputID, key := toolapp.DepthOutputIdentity(depthJob)
	output := depthJobOutput(depth)
	asset := mediadomain.MediaAsset{ID: outputID, ProjectID: project, Kind: mediadomain.KindVideo, Origin: mediadomain.OriginSystem, Status: mediadomain.StatusProcessing, ModerationStatus: mediadomain.ModerationPending, Revision: 1, ObjectKey: key, FileName: "depth.mp4", MimeType: output.File.MIMEType, ByteSize: output.File.Size, SHA256: &output.File.SHA256, Width: output.Probe.Width, Height: output.Probe.Height, DurationMS: output.Probe.DurationMS, FPS: output.Probe.FPS, Codec: output.Probe.Codec, CreateTime: now, UpdateTime: now}
	artifact := toolapp.DepthArtifact{JobID: depthID, Attempt: 1, InputSHA256: digest, Receipt: output.Receipt, Asset: asset, Objects: []toolapp.DepthObject{{Kind: "original", ObjectKey: key, SHA256: output.File.SHA256, ByteSize: output.File.Size, MIMEType: output.File.MIMEType}}}
	for _, rendition := range []struct {
		Kind      mediadomain.RenditionKind
		Ext, MIME string
	}{{mediadomain.RenditionPoster, "png", "image/png"}, {mediadomain.RenditionProxy720p, "mp4", "video/mp4"}} {
		objectKey := key[:len(key)-4] + "/" + string(rendition.Kind) + "." + rendition.Ext
		size, w, h := int64(128), int32(64), int32(64)
		artifact.Renditions = append(artifact.Renditions, mediadomain.Rendition{ID: uuid.NewSHA1(outputID, []byte(rendition.Kind)), MediaAssetID: outputID, Kind: rendition.Kind, ObjectKey: objectKey, Width: &w, Height: &h, ByteSize: &size, CreateTime: now, UpdateTime: now})
		artifact.Objects = append(artifact.Objects, toolapp.DepthObject{Kind: string(rendition.Kind), ObjectKey: objectKey, SHA256: digest, ByteSize: size, MIMEType: rendition.MIME})
	}
	artifactSHA, artifactBody, err := toolapp.DepthArtifactDigest(depthJob, depth, artifact)
	if err != nil {
		t.Fatal("typed metadata fixture", err)
	}
	if err := database.Exec(`INSERT INTO mediatool.depth_result_intent(job_id,attempt,asset_id,output_sha256,artifact_sha256,artifact,created_at)VALUES(?,1,?,?,?,?::jsonb,?)`, depthID, outputID, output.File.SHA256, artifactSHA, string(artifactBody), now).Error; err != nil {
		t.Fatal(err)
	}
	for i, object := range artifact.Objects {
		if err := database.Exec(`INSERT INTO mediatool.depth_object(job_id,attempt,kind,object_key,sha256,byte_size,mime_type,write_started,status,updated_at)VALUES(?,1,?,?,?,?,?,?,'pending',?)`, depthID, object.Kind, object.ObjectKey, object.SHA256, object.ByteSize, object.MIMEType, i == 0, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Exec(`UPDATE mediatool.export_job SET status='cancelled',stage='cancelled',attempt=2 WHERE id=?`, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{source.ID, video.AssetID, outputID} {
		if err := database.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
				return err
			}
			reader := toolpg.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx)))
			found, err := reader.HasMediaReferences(t.Context(), actor, project, id)
			if err != nil || !found {
				t.Fatal("lost retained source or pending artifact identity", found, err)
			}
			keys, err := reader.StorageUsageKeys(t.Context(), actor, project)
			if err != nil || !slices.Contains(keys, key) || !slices.Contains(keys, video.ObjectKey) || !slices.Contains(keys, source.ObjectKey) || len(keys) != 11 {
				t.Fatal("incomplete exact owning keys", len(keys), err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMediaToolRetainedReaderRejectsUnknownHistoryWithoutSubtotal(t *testing.T) {
	database, actor, project := retainedToolDatabase(t)
	id := uuid.New()
	if err := database.Exec(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at)VALUES(?,?,?,?,'producer',?,?,1,'{"unknown_media":"not-an-empty-proof"}','cancelled','cancelled',0,1,1,?,?)`, id, project, actor.OrgID, actor.ID, uuid.New(), uuid.New(), time.Now(), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		reader := toolpg.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx)))
		if found, err := reader.HasMediaReferences(t.Context(), actor, project, uuid.New()); err == nil || found {
			t.Fatal("unknown frozen history became no references", found, err)
		}
		if keys, err := reader.StorageUsageKeys(t.Context(), actor, project); err == nil || keys != nil {
			t.Fatal("unknown frozen history became usage subtotal", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMediaToolRetainedReaderRequiresCurrentAuthorityAndCallerTransaction(t *testing.T) {
	database, actor, project := retainedToolDatabase(t)
	if _, err := toolpg.NewRetainedMediaReader(database, mediaapp.NewDerivedService(pgmedia.NewStore(database))).StorageUsageKeys(t.Context(), actor, project); err == nil {
		t.Fatal("pool accepted")
	}
	foreign, _ := mediaStoreProject(t, database)
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		if _, err := toolpg.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx))).StorageUsageKeys(t.Context(), foreign, project); !errors.Is(err, mediaapp.ErrNotFound) {
			t.Fatal("foreign project", err)
		}
		if _, err := toolpg.NewRetainedMediaReader(tx, nil).StorageUsageKeys(t.Context(), actor, project); err == nil {
			t.Fatal("missing owner treated as empty")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		_, err := toolpg.NewRetainedMediaReader(tx, mediaapp.NewDerivedService(pgmedia.NewStore(tx))).StorageUsageKeys(t.Context(), actor, project)
		if !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatal("revoked current actor", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
