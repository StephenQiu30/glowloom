package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	objects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type routedDepthSource struct{ asset uuid.UUID }

func (s routedDepthSource) FreezeTranscriptionSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error) {
	return s.asset, nil
}

func routedDepthStore(db *gorm.DB, source uuid.UUID, worker bool) *pgtool.DepthStore {
	return pgtool.NewDepthStore(db, func(*gorm.DB) toolapp.TranscriptionSourceReader {
		return routedDepthSource{asset: source}
	}, func(tx *gorm.DB) toolapp.DepthDerivedMedia {
		if worker {
			return mediaapp.NewDerivedService(mediaWorkerStore(tx))
		}
		return mediaapp.NewDerivedService(mediaTestStore(tx))
	}).WithStorageAdmission(func(tx *gorm.DB) toolapp.ToolStorageLedger { return pgmedia.NewStorageRoutes(tx) })
}

// The typed native receipt is controlled metadata, not an inference claim. This
// test isolates SQL ownership/atomicity and real native-SDK byte storage.
func routedDepthArtifact(t *testing.T, job tooldomain.DepthJob, frozen tooldomain.FrozenDepth) (toolapp.DepthArtifact, [][]byte) {
	t.Helper()
	data := [][]byte{[]byte("controlled-depth-original"), []byte("controlled-depth-poster"), []byte("controlled-depth-proxy")}
	output := depthJobOutput(frozen)
	output.Receipt.Source.Width, output.Receipt.Source.Height = *frozen.Input.Width, *frozen.Input.Height
	output.Receipt.Prepared = output.Receipt.Source
	id, key := toolapp.DepthOutputIdentity(job)
	now := time.Now().UTC().Truncate(time.Microsecond)
	sha := sha256.Sum256(data[0])
	digest := hex.EncodeToString(sha[:])
	a := toolapp.DepthArtifact{JobID: job.ID, Attempt: job.Attempt, InputSHA256: frozen.Input.SHA256, Receipt: output.Receipt,
		Asset: mediadomain.MediaAsset{ID: id, ProjectID: job.ProjectID, Kind: mediadomain.KindVideo, Origin: mediadomain.OriginSystem, Status: mediadomain.StatusProcessing, ModerationStatus: mediadomain.ModerationPending, Revision: 1, ObjectKey: key, FileName: "depth.mp4", MimeType: "video/mp4", ByteSize: int64(len(data[0])), SHA256: &digest, Width: output.Probe.Width, Height: output.Probe.Height, DurationMS: output.Probe.DurationMS, FPS: output.Probe.FPS, Codec: output.Probe.Codec, CreateTime: now, UpdateTime: now}}
	for i, kind := range []string{"original", "poster", "proxy_720p"} {
		mime, objectKey := "video/mp4", key
		if i > 0 {
			ext := "mp4"
			if kind == "poster" {
				ext, mime = "png", "image/png"
			}
			objectKey = strings.TrimSuffix(key, path.Ext(key)) + "/" + kind + "." + ext
			size, width, height := int64(len(data[i])), int32(64), int32(64)
			a.Renditions = append(a.Renditions, mediadomain.Rendition{ID: uuid.NewSHA1(id, []byte(kind)), MediaAssetID: id, Kind: mediadomain.RenditionKind(kind), ObjectKey: objectKey, Width: &width, Height: &height, ByteSize: &size, CreateTime: now, UpdateTime: now})
		}
		sum := sha256.Sum256(data[i])
		a.Objects = append(a.Objects, toolapp.DepthObject{Kind: kind, ObjectKey: objectKey, SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(data[i])), MIMEType: mime})
	}
	if _, _, err := toolapp.DepthArtifactDigest(job, frozen, a); err != nil {
		t.Fatal("controlled artifact fixture", err)
	}
	return a, data
}

func TestDepthRoutedRealPostgresNativeTOSAtomicLocationsAndRecovery(t *testing.T) {
	owner, _ := routedGeneratedDatabase(t)
	tx := owner.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	actor, project := mediaStoreProject(t, tx)
	registry, writes, _ := controlledGeneratedTOS(t)
	routes := pgmedia.NewStorageRoutes(tx)
	profile := mediaapp.StorageProfile{ID: "tos-frozen", Provider: "tos", Endpoint: "https://tos-fixture.example.invalid", SigningEndpoint: "https://tos-fixture.example.invalid", Region: "cn-beijing", Bucket: "controlled-fixture"}
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}); err != nil {
		t.Fatal(err)
	}
	sourceOwner := mediaapp.StorageWorkOwner{Kind: "operation", OwnerID: project, WorkID: uuid.New()}
	if err := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision) VALUES(?,'operation',?,?,'tos-frozen','controlled-fixture','controlled-v1')`, uuid.New(), project, sourceOwner.WorkID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	factory := toolflow.NewRoutedObjects(registry, routes, nil)
	sourceObjects, err := factory.Open(t.Context(), sourceOwner, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceFile := depthTestVideo(t, "30", "2")
	sourceBytes, err := io.ReadAll(sourceFile.File)
	if err != nil {
		t.Fatal(err)
	}
	source := imageAsset(project, uuid.New())
	source.Kind, source.MimeType, source.FileName = mediadomain.KindVideo, "video/mp4", "source.mp4"
	source.ObjectKey = "projects/" + project.String() + "/video/2026/10/" + source.ID.String() + ".mp4"
	source.ByteSize, source.SHA256 = int64(len(sourceBytes)), &sourceFile.SHA256
	width, height, duration := int32(320), int32(180), int32(2000)
	source.Width, source.Height, source.DurationMS = &width, &height, &duration
	if err := sourceObjects.PutIfAbsent(t.Context(), source.ObjectKey, bytes.NewReader(sourceBytes), source.ByteSize, source.MimeType, *source.SHA256); err != nil {
		t.Fatal(err)
	}
	sourceIntent, _ := routes.ReadStorageWork(t.Context(), sourceOwner)
	location, _, err := routes.FindWorkLocation(t.Context(), sourceIntent.ID, source.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := routes.AcceptStorageWork(t.Context(), sourceIntent.ID, []uuid.UUID{location.ID}); err != nil {
		t.Fatal(err)
	}
	location.State = "accepted"
	source.ObjectLocation = &location
	// The source owner fixture is already read back through the native SDK; no
	// legacy CreateAsset path may omit its initial physical binding.
	if err := tx.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,moderation_status,revision,create_time,update_time,object_location_id) VALUES(?,?,'video','upload','ready',?,?,?,?,?,?,?,?,'passed',1,?,?,?)`, source.ID, project, source.ObjectKey, source.FileName, source.MimeType, source.ByteSize, source.SHA256, width, height, duration, source.CreateTime, source.UpdateTime, location.ID).Error; err != nil {
		t.Fatal(err)
	}
	store := routedDepthStore(tx, source.ID, false)
	workerStore := routedDepthStore(tx, source.ID, true)
	input := toolapp.DepthCreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1}
	// This test-owned outer transaction rolls back route changes and all fixtures.
	if err := tx.Exec(`RESET ROLE`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`DELETE FROM media.storage_route`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(t.Context(), actor, project, uuid.New(), input); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("empty configuration admitted depth", err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds) VALUES('controlled-depth-v1',repeat('c',64),'tos-frozen','{}'::jsonb,86400) ON CONFLICT DO NOTHING`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'controlled-depth-v1','tos-frozen')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	job, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil {
		t.Fatal("routed admission", err)
	}
	replay, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil || replay.ID != job.ID {
		t.Fatal("permanent admission receipt", err)
	}
	workID := toolapp.DepthWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()}
	work, err := workerStore.Claim(t.Context(), workID)
	if err != nil || len(work.SourceAssets) != 1 || work.SourceAssets[0].ObjectLocation == nil || work.SourceAssets[0].ObjectLocation.ID != location.ID {
		t.Fatal("claim lost source physical identity", err)
	}
	workOwner := toolapp.StorageOwner("depth", project, job.ID, 1)
	physical, err := factory.Open(t.Context(), workOwner, work.SourceAssets)
	if err != nil {
		t.Fatal(err)
	}
	read, err := physical.Get(t.Context(), source.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(read)
	closeErr := read.Close()
	if err != nil || closeErr != nil || !bytes.Equal(got, sourceBytes) {
		t.Fatal("source SDK readback", err, closeErr)
	}
	if err := workerStore.StartProcess(t.Context(), workID); err != nil {
		t.Fatal(err)
	}
	if err := workerStore.EndProcess(t.Context(), workID); err != nil {
		t.Fatal(err)
	}
	artifact, data := routedDepthArtifact(t, job, work.Frozen)
	digest, _, _ := toolapp.DepthArtifactDigest(job, work.Frozen, artifact)
	if err := workerStore.FreezeArtifact(t.Context(), workID, artifact); err != nil {
		t.Fatal("reserve location in first object INSERT", err)
	}
	rows, err := workerStore.Objects(t.Context(), workID)
	if err != nil || len(rows) != 3 {
		t.Fatal(err)
	}
	for i, object := range rows {
		if object.ObjectLocationID == nil {
			t.Fatal("missing initial physical binding")
		}
		if err := workerStore.BeginWrite(t.Context(), workID, object.ObjectKey); err != nil {
			t.Fatal(err)
		}
		if err := physical.PutIfAbsent(t.Context(), object.ObjectKey, bytes.NewReader(data[i]), object.ByteSize, object.MIMEType, object.SHA256); err != nil {
			t.Fatal("native SDK write", err)
		}
		reader, err := physical.Get(t.Context(), object.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil || !bytes.Equal(body, data[i]) {
			t.Fatal("complete byte verification", err, closeErr)
		}
		if err := workerStore.ConfirmObject(t.Context(), workID, object.ObjectKey); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if _, err := workerStore.Commit(t.Context(), workID); !errors.Is(err, toolapp.ErrConflict) {
				t.Fatal("partial artifact was published", err)
			}
		}
	}
	rollback := errors.New("rollback controlled extra-object branch")
	if err := tx.Transaction(func(inner *gorm.DB) error {
		intent, err := pgmedia.NewStorageRoutes(inner).ReadStorageWork(t.Context(), workOwner)
		if err != nil {
			return err
		}
		if _, err := pgmedia.NewStorageRoutes(inner).ReserveWorkLocation(t.Context(), intent.ID, artifact.Asset.ObjectKey+"/extra"); err != nil {
			return err
		}
		if _, err := routedDepthStore(inner, source.ID, true).Commit(t.Context(), workID); err == nil {
			t.Fatal("incomplete owned location set accepted")
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	result, err := workerStore.Commit(t.Context(), workID)
	if err != nil || result.Status != tooldomain.DepthReviewRequired || result.AssetID == nil || *writes != 4 {
		t.Fatal("atomic depth acceptance", err)
	}
	// Commit releases the execution fence; read the immutable JSON independently.
	var frozenRow struct{ Artifact []byte }
	if err := tx.Raw(`SELECT artifact FROM mediatool.depth_result_intent WHERE job_id=? AND attempt=1`, job.ID).Scan(&frozenRow).Error; err != nil {
		t.Fatal(err)
	}
	var saved toolapp.DepthArtifact
	if err := json.Unmarshal(frozenRow.Artifact, &saved); err != nil {
		t.Fatal(err)
	}
	after, _, err := toolapp.DepthArtifactDigest(job, work.Frozen, saved)
	if err != nil || after != digest || saved.Asset.ObjectLocation != nil {
		t.Fatal("physical metadata changed permanent JSON digest", err)
	}
	var bindings int64
	if err := tx.Raw(`SELECT count(*) FROM mediatool.depth_object d JOIN media.object_location l ON l.id=d.object_location_id WHERE d.job_id=? AND l.state='accepted'`, job.ID).Scan(&bindings).Error; err != nil || bindings != 3 {
		t.Fatal("partial physical acceptance", err)
	}
	reader := objects.NewRoutedReadObjects(registry)
	query := toolapp.NewRoutedDepthQuery(store, reader, reader, nil, nil)
	if _, err := query.Preview(t.Context(), actor, project, job.ID); err != nil {
		t.Fatal("exact pending preview", err)
	}
	if _, _, err := query.Download(t.Context(), actor, project, job.ID); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("pending bytes downloadable", err)
	}
	if err := tx.Transaction(func(inner *gorm.DB) error {
		cancelStore := routedDepthStore(inner, source.ID, false)
		cancelled, err := cancelStore.Control(t.Context(), actor, project, job.ID, uuid.New(), result.Revision, "cancel")
		if err != nil {
			return err
		}
		worker := toolapp.NewRoutedDepthWorker(routedDepthStore(inner, source.ID, true), toolflow.NewRoutedObjects(registry, pgmedia.NewStorageRoutes(inner), nil), nil, nil, nil, nil)
		failed, err := worker.Execute(t.Context(), toolapp.DepthWorkID{JobID: job.ID, Attempt: cancelled.Attempt, ExecutionID: uuid.New()})
		if err == nil || !failed.NeedsReconciliation || failed.Status == tooldomain.DepthCancelled {
			t.Fatal("accepted artifact was falsely removed", err)
		}
		asset, err := mediaapp.NewDerivedService(mediaTestStore(inner)).Asset(t.Context(), actor, project, *result.AssetID)
		if err != nil || asset.CanReference() {
			t.Fatal("cancelled pending artifact became referenceable", err)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	result, err = store.Review(t.Context(), actor, job.ID, uuid.New(), toolapp.ReviewInput{ProjectID: project, Revision: result.Revision, SHA256: *result.SHA256, LocalReviewConfirmed: true})
	if err != nil || result.Status != tooldomain.DepthSucceeded {
		t.Fatal(err)
	}
	file, size, err := query.Download(t.Context(), actor, project, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(file)
	closeErr = file.Close()
	if err != nil || closeErr != nil || size != int64(len(data[0])) || !bytes.Equal(body, data[0]) {
		t.Fatal("final native SDK bytes", err, closeErr)
	}
	second, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	secondID := toolapp.DepthWorkID{JobID: second.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := workerStore.Claim(t.Context(), secondID); err != nil {
		t.Fatal(err)
	}
	second, err = workerStore.Finish(t.Context(), secondID, true, false, "dependency_unavailable")
	if err != nil || !second.Retryable {
		t.Fatal(err)
	}
	if err := tx.Exec(`RESET ROLE`).Error; err != nil {
		t.Fatal(err)
	}
	nextProfile := profile
	nextProfile.ID = "depth-next-" + uuid.NewString()[:8]
	nextProfile.Bucket = "controlled-next-" + uuid.NewString()[:8]
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{nextProfile}); err != nil {
		t.Fatal(err)
	}
	revision := "controlled-depth-" + uuid.NewString()
	if err := tx.Exec(`INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds) VALUES(?,repeat('d',64),?,'{}'::jsonb,86400)`, revision, nextProfile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE media.storage_route SET revision=?,profile_id=? WHERE singleton=1`, revision, nextProfile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
		t.Fatal(err)
	}
	retried, err := store.Control(t.Context(), actor, project, second.ID, uuid.New(), second.Revision, "retry")
	if err != nil || retried.Attempt != 2 {
		t.Fatal("retry admission", err)
	}
	priorIntent, err := routes.ReadStorageWork(t.Context(), toolapp.StorageOwner("depth", project, second.ID, 1))
	if err != nil {
		t.Fatal(err)
	}
	nextIntent, err := routes.ReadStorageWork(t.Context(), toolapp.StorageOwner("depth", project, second.ID, 2))
	if err != nil || priorIntent.ProfileID != "tos-frozen" || nextIntent.ProfileID != nextProfile.ID || priorIntent.ID == nextIntent.ID {
		t.Fatal("retry moved the old attempt or reused its physical namespace", err)
	}
}
