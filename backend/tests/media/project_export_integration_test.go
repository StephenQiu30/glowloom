package media_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// This adapter stores actual bytes and simulates one lost Put receipt. It is not S3 acceptance.
type projectObjects struct {
	mu          sync.Mutex
	data        map[string][]byte
	info        map[string]mediaapp.ObjectInfo
	loseReceipt bool
	puts        int
}

func (o *projectObjects) Get(_ context.Context, key string) (io.ReadCloser, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.data[key]
	if !ok {
		return nil, toolapp.ErrObjectMissing
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (o *projectObjects) Stat(_ context.Context, key string) (mediaapp.ObjectInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	info, ok := o.info[key]
	if !ok {
		return info, toolapp.ErrObjectMissing
	}
	return info, nil
}
func (o *projectObjects) PutIfAbsent(_ context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.data[key]; ok {
		return mediaapp.ErrObjectAlreadyExists
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	o.data[key] = data
	o.info[key] = mediaapp.ObjectInfo{Size: size, ContentType: mime, SHA256: digest}
	o.puts++
	if o.loseReceipt {
		o.loseReceipt = false
		return errors.New("unknown object PUT result")
	}
	return nil
}
func (o *projectObjects) PresignGet(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "https://objects.test.invalid/review.zip", nil
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
	db := mediaStoreDB(t)
	owner := projectOwnerDB(t)
	actor, projectID := mediaStoreProject(t, owner)
	_, foreign := mediaStoreProject(t, owner)
	frozen, paths := projectAVFixture(t)
	objects := &projectObjects{data: map[string][]byte{}, info: map[string]mediaapp.ObjectInfo{}, loseReceipt: true}
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
		objects.data[asset.ObjectKey] = data
		objects.info[asset.ObjectKey] = mediaapp.ObjectInfo{Size: asset.ByteSize, ContentType: asset.MimeType, SHA256: input.SHA256}
	}
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaTestStore(tx), nil) }
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
	store := toolpg.NewStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(mediaCanvasStore(tx, mediaFactory))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	workerStore := toolpg.NewStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(mediaCanvasWorkerStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaWorkerStore(tx), nil) }))
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
	worker := toolapp.NewWorker(workerStore, objects, project.NewRenderer(nil), mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	activities := toolflow.NewActivities(worker, workerStore, workerStore)
	env.RegisterWorkflow(toolflow.ExportWorkflow)
	env.RegisterActivityWithOptions(activities.RenderExport, activity.RegisterOptions{Name: toolflow.RenderActivity})
	env.RegisterActivityWithOptions(activities.FailExportWorkflow, activity.RegisterOptions{Name: toolflow.FailureActivity})
	env.ExecuteWorkflow(toolflow.ExportWorkflow, toolapp.WorkID{JobID: job.ID, Attempt: 1})
	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatal(env.GetWorkflowError())
	}
	var output tool.ExportJob
	if err = env.GetWorkflowResult(&output); err != nil || output.Status != tool.ReviewRequired || output.AssetID != nil || output.ArtifactID == nil || output.SHA256 == nil {
		t.Fatalf("artifact lifecycle %+v %v", output, err)
	}
	if objects.puts != 1 {
		t.Fatalf("unknown retry rewrote output %d", objects.puts)
	}
	query := toolapp.NewExportQuery(store, objects, objects)
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
