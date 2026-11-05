package media_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	tooltask "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// projectUnknownReceiptFactory loses one reply after the real routed provider
// write, preserving byte readback and permanent ledger proof on task retry.
type projectUnknownReceiptFactory struct {
	toolapp.WorkObjectsFactory
	loseReceipt bool
	puts        int
}

func (f *projectUnknownReceiptFactory) Open(ctx context.Context, owner mediaapp.StorageWorkOwner, inputs []mediadomain.MediaAsset) (toolapp.DepthObjects, error) {
	o, err := f.WorkObjectsFactory.Open(ctx, owner, inputs)
	if err != nil {
		return nil, err
	}
	return &projectUnknownReceiptObjects{DepthObjects: o, factory: f}, nil
}

type projectUnknownReceiptObjects struct {
	toolapp.DepthObjects
	factory *projectUnknownReceiptFactory
}

func (o *projectUnknownReceiptObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	err := o.DepthObjects.PutIfAbsent(ctx, key, r, size, mime, digest)
	if err == nil {
		o.factory.puts++
		if o.factory.loseReceipt {
			o.factory.loseReceipt = false
			return io.ErrUnexpectedEOF
		}
	}
	return err
}
func projectOwnerDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_PROJECT_EXPORT_OWNER_DSN")
	if dsn == "" {
		t.Skip("set isolated project-export owner DSN for fixture creation")
	}
	conn, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB
}
func TestProjectExportDurableFrozenCanvasArtifactReviewAndUnknownPutRecovery(t *testing.T) {
	db := mediatoolFixtureDB(t)
	owner := projectOwnerDB(t)
	actor, projectID := mediaStoreProject(t, owner)
	_, foreign := mediaStoreProject(t, owner)
	frozen, paths := projectAVFixture(t)
	objects := &projectUnknownReceiptFactory{WorkObjectsFactory: mediatoolRoutedObjects(t, db), loseReceipt: true}
	provider := glbTestObjects(t)
	for _, input := range frozen.Inputs {
		asset := imageAsset(projectID, input.AssetID)
		asset.Kind = mediadomain.Kind(input.Kind)
		asset.Width, asset.Height, asset.ByteSize, asset.SHA256 = input.Width, input.Height, input.ByteSize, &input.SHA256
		asset.DurationMS, asset.FPS = input.DurationMS, input.FPS
		asset.AudioChannels = input.AudioChannels
		// Exercise historical video rows whose old prober omitted the audio stream.
		if input.Kind == "video" {
			asset.AudioChannels = nil
		}
		asset.MimeType = input.MIMEType
		ext, _ := toolapp.ProjectMediaExtension(input.MIMEType)
		asset.FileName = "source" + ext
		asset.ObjectKey = "projects/" + projectID.String() + "/" + input.Kind + "/2026/10/" + input.AssetID.String() + ext
		asset.Status, asset.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
		asset = mediaFixtureAssetLocation(t, db, asset)
		if err := mediaTestStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
			t.Fatal(err)
		}
		if err := owner.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(paths[input.AssetID])
		if err != nil {
			t.Fatal(err)
		}
		if err := provider.Put(t.Context(), asset.ObjectKey, bytes.NewReader(data), int64(len(data)), asset.MimeType); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = provider.Remove(context.Background(), asset.ObjectKey) })
	}
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaTestStore(tx)) }
	canvasService := canvasfixture.NewService(mediaCanvasStore(db, mediaFactory))
	doc, err := canvasService.Create(t.Context(), actor, projectID, uuid.NewString(), canvasapp.CreateInput{Name: "Project export fixture"})
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), Title: "Timeline", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &frozen.Timeline}}
	saved, err := canvasService.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: doc.Revision, Operations: canvasfixture.Concat(canvasfixture.Nodes([]canvasdomain.Node{node}))})
	if err != nil {
		t.Fatal(err)
	}
	store := mediatoolExportStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(mediaCanvasStore(tx, mediaFactory))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	workerStore := mediatoolExportStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(mediaCanvasWorkerStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaWorkerStore(tx)) }))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaWorkerStore(tx)) })
	request := toolapp.CreateInput{OutputKind: tool.OutputFCPXML, CanvasID: doc.ID, NodeID: node.ID, Revision: saved.Revision}
	key := uuid.New()
	job, err := store.Create(t.Context(), actor, projectID, key, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Create(t.Context(), actor, projectID, key, request)
	if err != nil || again.ID != job.ID {
		t.Fatalf("create replay %+v %v", again, err)
	}
	stale := request
	stale.Revision--
	if _, err = store.Create(t.Context(), actor, projectID, uuid.New(), stale); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("stale canvas accepted %v", err)
	}
	worker := toolapp.NewRoutedWorker(workerStore, objects, project.NewRenderer(nil), mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
	taskfixture.RunWork(t, db, tooltask.Export, map[string]any{"job_id": job.ID}, tooltask.ExportHandler(worker, workerStore))
	output, err := store.Get(t.Context(), actor, projectID, job.ID)
	if err != nil || output.Status != tool.ReviewRequired || output.AssetID != nil || output.ArtifactID == nil || output.SHA256 == nil {
		t.Fatalf("artifact lifecycle %+v %v", output, err)
	}
	if objects.puts != 1 {
		t.Fatalf("unknown retry rewrote output %d", objects.puts)
	}
	query := toolapp.NewRoutedExportQuery(store, mediaFixtureReadObjects(t), mediaFixtureReadObjects(t), 5*time.Minute)
	preview, err := query.Preview(t.Context(), actor, projectID, job.ID)
	if err != nil || preview.Artifact == nil || preview.Artifact.ID != *output.ArtifactID || preview.SHA256 != *output.SHA256 {
		t.Fatalf("artifact preview %+v %v", preview, err)
	}
	if _, _, err = query.Download(t.Context(), actor, projectID, job.ID); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("unreviewed download %v", err)
	}
	if _, _, err = store.PreviewArtifact(t.Context(), actor, foreign, job.ID); !errors.Is(err, toolapp.ErrNotFound) {
		t.Fatalf("foreign artifact read %v", err)
	}
	for _, bad := range []toolapp.ReviewInput{
		{ProjectID: projectID, Revision: output.Revision, SHA256: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", LocalReviewConfirmed: true},
		{ProjectID: projectID, Revision: output.Revision - 1, SHA256: *output.SHA256, LocalReviewConfirmed: true},
	} {
		if _, e := store.Review(t.Context(), actor, job.ID, uuid.New(), bad); !errors.Is(e, toolapp.ErrConflict) {
			t.Fatalf("stale/hash review accepted %v", e)
		}
	}
	reviewed, err := store.Review(t.Context(), actor, job.ID, uuid.New(), toolapp.ReviewInput{ProjectID: projectID, Revision: output.Revision, SHA256: *output.SHA256, LocalReviewConfirmed: true})
	if err != nil || reviewed.Status != tool.Succeeded {
		t.Fatalf("review %+v %v", reviewed, err)
	}
	reader, size, err := query.Download(t.Context(), actor, projectID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || int64(len(data)) != size || !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatalf("not real ZIP %d %v", len(data), err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), size)
	if err != nil {
		t.Fatal(err)
	}
	videoAudioFound := false
	for _, entry := range archive.File {
		if entry.Name != "timeline.fcpxmld/Info.fcpxml" {
			continue
		}
		r, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		var doc struct {
			Assets []struct {
				UID      string `xml:"uid,attr"`
				HasAudio string `xml:"hasAudio,attr"`
				Channels int    `xml:"audioChannels,attr"`
			} `xml:"resources>asset"`
		}
		e = xml.NewDecoder(r).Decode(&doc)
		_ = r.Close()
		if e != nil {
			t.Fatal(e)
		}
		for _, original := range frozen.Inputs {
			if original.Kind != "video" {
				continue
			}
			for _, a := range doc.Assets {
				if a.UID == original.AssetID.String() && a.HasAudio == "1" && a.Channels == 2 {
					videoAudioFound = true
				}
			}
		}
	}
	if !videoAudioFound {
		t.Fatal("historical missing video audio metadata lost real stream")
	}
	fresh, err := store.Get(t.Context(), actor, projectID, job.ID)
	if err != nil || fresh.ArtifactID == nil || *fresh.ArtifactID != *output.ArtifactID || fresh.AssetID != nil {
		t.Fatalf("artifact readback %+v %v", fresh, err)
	}
	// A cancelled inspection result cannot be previewed or adopted by the next attempt.
	pending, err := store.Create(t.Context(), actor, projectID, uuid.New(), request)
	if err != nil {
		t.Fatal(err)
	}
	workID := toolapp.WorkID{JobID: pending.ID, Attempt: 1, ExecutionID: uuid.New()}
	produced, err := worker.Execute(t.Context(), workID)
	if err != nil || produced.Status != tool.ReviewRequired {
		t.Fatalf("second output %+v %v", produced, err)
	}
	_, artifact, err := store.PreviewArtifact(t.Context(), actor, projectID, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.Control(t.Context(), actor, projectID, pending.ID, uuid.New(), produced.Revision, "cancel")
	if err != nil || cancelled.Status != tool.Cancelled {
		t.Fatalf("cancel %+v %v", cancelled, err)
	}
	if _, _, err = store.PreviewArtifact(t.Context(), actor, projectID, pending.ID); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("cancelled preview %v", err)
	}
	if _, err = workerStore.CommitArtifact(t.Context(), workID, artifact); !errors.Is(err, toolapp.ErrCancelled) {
		t.Fatalf("late cancelled commit %v", err)
	}
	retry, err := store.Control(t.Context(), actor, projectID, pending.ID, uuid.New(), cancelled.Revision, "retry")
	if err != nil || retry.Attempt != 2 || retry.ArtifactID != nil || retry.SHA256 != nil {
		t.Fatalf("retry identity %+v %v", retry, err)
	}
	if _, err = workerStore.CommitArtifact(t.Context(), workID, artifact); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatalf("old attempt commit %v", err)
	}
	replacement, err := worker.Execute(t.Context(), toolapp.WorkID{JobID: pending.ID, Attempt: 2, ExecutionID: uuid.New()})
	if err != nil || replacement.ArtifactID == nil || *replacement.ArtifactID == artifact.ID {
		t.Fatalf("replacement identity %+v %v", replacement, err)
	}
	var fakeAssets int64
	if err = owner.Raw(`SELECT count(*) FROM media.media_asset WHERE id=?`, *output.ArtifactID).Scan(&fakeAssets).Error; err != nil || fakeAssets != 0 {
		t.Fatal("engineering ZIP registered as fake media")
	}
	var acl struct {
		Role      string
		CanUpdate bool
	}
	if err = db.Raw(`SELECT current_role AS role,has_table_privilege(current_role,'mediatool.export_artifact','UPDATE') AS can_update`).Scan(&acl).Error; err != nil || acl.Role != "glowloom_app" || acl.CanUpdate {
		t.Fatalf("artifact ACL %+v %v", acl, err)
	}
}
