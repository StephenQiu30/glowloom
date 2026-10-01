package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	toolhttp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/http"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestExportAudioRealHTTPPrivateM4AWaveformReviewAndDownload(t *testing.T) {
	db := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	inputPath := filepath.Join(t.TempDir(), "source.mp4")
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=25:d=0.6", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.6", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "0.6", inputPath).CombinedOutput(); err != nil {
		t.Fatalf("create actual upload source: %v %s", err, output)
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	download := &mediaapp.Downloaded{File: file, Size: int64(len(data)), MIMEType: "video/mp4", SHA256: hex.EncodeToString(digest[:])}
	defer func() { _ = download.Close() }()
	mediaStore := mediapg.NewStore(db)
	upload := mediaapp.NewUploadService(mediaStore, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	uploaded, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "source.mp4"}, File: download, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var keys []string
		if err := db.WithContext(ctx).Raw(`SELECT object_key FROM media.media_asset WHERE project_id=? UNION ALL SELECT r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE a.project_id=?`, project, project).Scan(&keys).Error; err != nil {
			t.Error(err)
			return
		}
		for _, key := range keys {
			if err := objects.Remove(ctx, key); err != nil {
				t.Error("clean exact audio test object")
			}
		}
	})
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }
	canvasService := canvasapp.NewService(canvaspg.NewStore(db, mediaFactory))
	doc, err := canvasService.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "Audio export actual fixture"})
	if err != nil {
		t.Fatal(err)
	}
	config := exportTimeline(t)
	config.Tracks[0].Kind, config.Clips[0].Kind = "video", "video"
	config.Clips[0].AssetID, config.Clips[0].DurationMS = &uploaded.Asset.ID, 400
	config.Clips[0].SourceStartMS, config.Clips[0].FadeInMS, config.Clips[0].FadeOutMS = 100, 100, 100
	node := canvasdomain.Node{ID: uuid.New(), Title: "Audio timeline", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &config}}
	saved, err := canvasService.Execute(t.Context(), actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: doc.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{node}}}})
	if err != nil {
		t.Fatal(err)
	}
	store := toolpg.NewStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(canvaspg.NewStore(tx, mediaFactory))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	toolhttp.NewHandler(store, toolapp.NewExportQuery(store, objects, objects)).Register(g)
	request := func(method, url string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var raw []byte
		if body != nil {
			var err error
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:3000")
		if method == http.MethodPost {
			req.Header.Set("Idempotency-Key", uuid.NewString())
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	created := request(http.MethodPost, "/api/projects/"+project.String()+"/media-exports", map[string]any{"canvas_id": doc.ID, "node_id": node.ID, "revision": saved.Revision, "output_kind": "audio"})
	if created.Code != http.StatusAccepted {
		t.Fatalf("actual audio create: %d %s", created.Code, created.Body.String())
	}
	var job tooldomain.ExportJob
	if err := json.Unmarshal(created.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	renderer, err := toolff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	worker := toolapp.NewWorker(store, toolflow.NewObjects(objects), renderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
	activities := toolflow.NewActivities(worker, store, store)
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(activities.RenderExport, activity.RegisterOptions{Name: toolflow.RenderActivity})
	env.RegisterActivityWithOptions(activities.FailExportWorkflow, activity.RegisterOptions{Name: toolflow.FailureActivity})
	env.ExecuteWorkflow(toolflow.ExportWorkflow, toolapp.WorkID{JobID: job.ID, Attempt: 1})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	var rendered tooldomain.ExportJob
	if err := env.GetWorkflowResult(&rendered); err != nil || rendered.Status != tooldomain.ReviewRequired || rendered.AssetID == nil {
		t.Fatalf("actual pending audio: %+v %v", rendered, err)
	}
	base := "/api/media-exports/" + job.ID.String()
	scope := "?project_id=" + project.String()
	previewed := request(http.MethodGet, base+"/preview"+scope, nil)
	var preview toolapp.ExportPreview
	if previewed.Code != http.StatusOK || json.Unmarshal(previewed.Body.Bytes(), &preview) != nil || preview.Asset.Kind != "audio" || preview.Asset.MIMEType != "audio/mp4" || preview.Asset.Width != nil || preview.Asset.Height != nil {
		t.Fatalf("actual audio preview: %d %s", previewed.Code, previewed.Body.String())
	}
	var previewShape struct {
		Waveform *struct {
			URL    string `json:"url"`
			Width  int32  `json:"width"`
			Height int32  `json:"height"`
		} `json:"waveform"`
	}
	if err := json.Unmarshal(previewed.Body.Bytes(), &previewShape); err != nil || previewShape.Waveform == nil || previewShape.Waveform.Width != 1280 || previewShape.Waveform.Height != 256 {
		t.Fatalf("pending job did not expose its actual waveform: %v", err)
	}
	waveRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, previewShape.Waveform.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	waveResponse, err := http.DefaultClient.Do(waveRequest)
	if err != nil {
		t.Fatal(err)
	}
	waveImage, waveErr := png.Decode(io.LimitReader(waveResponse.Body, 1<<20))
	_ = waveResponse.Body.Close()
	if waveResponse.StatusCode != http.StatusOK || waveErr != nil || waveImage.Bounds().Dx() != 1280 || waveImage.Bounds().Dy() != 256 {
		t.Fatalf("signed pending waveform was not actual PNG: %d %v", waveResponse.StatusCode, waveErr)
	}
	for _, mismatch := range []struct {
		project  uuid.UUID
		revision int64
		sha      string
	}{
		{uuid.New(), preview.Revision, preview.SHA256},
		{project, preview.Revision - 1, preview.SHA256},
		{project, preview.Revision, strings.Repeat("0", 64)},
	} {
		if _, err := store.PreviewWaveform(t.Context(), actor, mismatch.project, job.ID, mismatch.revision, mismatch.sha); err == nil {
			t.Fatal("waveform authorization accepted different project/revision/hash")
		}
	}
	if _, err := mediaapp.NewAssetQuery(mediaStore, objects).Reference(t.Context(), actor, project, *rendered.AssetID); err == nil {
		t.Fatal("unreviewed audio became a canvas reference")
	}
	if got := request(http.MethodGet, base+"/download"+scope, nil); got.Code != http.StatusConflict {
		t.Fatal("unreviewed audio allowed attachment download")
	}
	renditions, err := mediaStore.FindRenditions(t.Context(), actor, project, *rendered.AssetID)
	if err != nil || len(renditions) != 1 || renditions[0].Kind != mediadomain.RenditionWaveform {
		t.Fatalf("actual waveform metadata: %+v %v", renditions, err)
	}
	waveform, err := objects.Get(t.Context(), renditions[0].ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(io.LimitReader(waveform, 1<<20))
	_ = waveform.Close()
	if err != nil || image.Bounds().Dx() != 1280 || image.Bounds().Dy() != 256 {
		t.Fatalf("actual waveform bytes: %v", err)
	}
	review := request(http.MethodPost, base+"/review", toolapp.ReviewInput{ProjectID: project, Revision: preview.Revision, SHA256: preview.SHA256, LocalReviewConfirmed: true})
	if review.Code != http.StatusOK {
		t.Fatalf("actual audio review: %d %s", review.Code, review.Body.String())
	}
	result := request(http.MethodGet, base+"/download"+scope, nil)
	outputHash := sha256.Sum256(result.Body.Bytes())
	if result.Code != http.StatusOK || result.Header().Get("Content-Type") != "audio/mp4" || !strings.Contains(result.Header().Get("Content-Disposition"), ".m4a") || hex.EncodeToString(outputHash[:]) != preview.SHA256 {
		t.Fatalf("actual audio attachment: %d %s", result.Code, result.Header().Get("Content-Type"))
	}
	asset, err := mediaapp.NewAssetQuery(mediaStore, objects).Reference(t.Context(), actor, project, *rendered.AssetID)
	if err != nil || asset.Kind != "audio" {
		t.Fatalf("reviewed audio reference: %+v %v", asset, err)
	}

	// A real video lacking an audio stream must produce a durable failure,
	// without minting a silent audio asset or pretending media review succeeded.
	silentPath := filepath.Join(t.TempDir(), "silent.mp4")
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=64x64:r=25:d=0.2", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", silentPath).CombinedOutput(); err != nil {
		t.Fatalf("create actual silent source: %v %s", err, output)
	}
	silentBytes, err := os.ReadFile(silentPath)
	if err != nil {
		t.Fatal(err)
	}
	silentFile, err := os.Open(silentPath)
	if err != nil {
		t.Fatal(err)
	}
	silentHash := sha256.Sum256(silentBytes)
	silentInput := &mediaapp.Downloaded{File: silentFile, Size: int64(len(silentBytes)), MIMEType: "video/mp4", SHA256: hex.EncodeToString(silentHash[:])}
	defer func() { _ = silentInput.Close() }()
	silentAsset, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "silent.mp4"}, File: silentInput, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	silentConfig := exportTimeline(t)
	silentConfig.Tracks[0].Kind, silentConfig.Clips[0].Kind = "video", "video"
	silentConfig.Clips[0].AssetID = &silentAsset.Asset.ID
	silentNode := canvasdomain.Node{ID: uuid.New(), Title: "Silent extraction", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &silentConfig}}
	silentSaved, err := canvasService.Execute(t.Context(), actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: saved.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{silentNode}}}})
	if err != nil {
		t.Fatal(err)
	}
	createdSilent := request(http.MethodPost, "/api/projects/"+project.String()+"/media-exports", map[string]any{"canvas_id": doc.ID, "node_id": silentNode.ID, "revision": silentSaved.Revision, "output_kind": "audio"})
	var silentJob tooldomain.ExportJob
	if createdSilent.Code != http.StatusAccepted || json.Unmarshal(createdSilent.Body.Bytes(), &silentJob) != nil {
		t.Fatalf("queue silent extraction: %d %s", createdSilent.Code, createdSilent.Body.String())
	}
	silentEnv := suite.NewTestWorkflowEnvironment()
	silentEnv.RegisterActivityWithOptions(activities.RenderExport, activity.RegisterOptions{Name: toolflow.RenderActivity})
	silentEnv.RegisterActivityWithOptions(activities.FailExportWorkflow, activity.RegisterOptions{Name: toolflow.FailureActivity})
	silentEnv.ExecuteWorkflow(toolflow.ExportWorkflow, toolapp.WorkID{JobID: silentJob.ID, Attempt: 1})
	if silentEnv.GetWorkflowError() == nil {
		t.Fatal("silent source extraction reported successful")
	}
	silentState, err := store.Get(t.Context(), actor, project, silentJob.ID)
	if err != nil || silentState.Status != tooldomain.Failed || silentState.FailureCode == nil || *silentState.FailureCode != "no_audio_stream" || silentState.AssetID != nil {
		t.Fatalf("actual no-audio failure facts: %+v %v", silentState, err)
	}

	// Repeating a short owned source makes a bounded real export long enough to
	// observe asynchronous cancellation during local processing, without a mock.
	cancelConfig := exportTimeline(t)
	cancelConfig.Tracks[0].Kind = "video"
	cancelConfig.Clips = make([]canvasdomain.TimelineClip, 50)
	for i := range cancelConfig.Clips {
		cancelConfig.Clips[i] = canvasdomain.TimelineClip{ID: uuid.New(), TrackID: cancelConfig.Tracks[0].ID, Kind: "video", AssetID: &uploaded.Asset.ID, Title: "Cancellation source", StartMS: int64(i) * 600, DurationMS: 600, Volume: 1}
	}
	cancelNode := canvasdomain.Node{ID: uuid.New(), Title: "Cancel actual audio", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &cancelConfig}}
	cancelSaved, err := canvasService.Execute(t.Context(), actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: silentSaved.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{cancelNode}}}})
	if err != nil {
		t.Fatal(err)
	}
	cancelCreate := request(http.MethodPost, "/api/projects/"+project.String()+"/media-exports", map[string]any{"canvas_id": doc.ID, "node_id": cancelNode.ID, "revision": cancelSaved.Revision, "output_kind": "audio"})
	var cancelJob tooldomain.ExportJob
	if cancelCreate.Code != http.StatusAccepted || json.Unmarshal(cancelCreate.Body.Bytes(), &cancelJob) != nil {
		t.Fatalf("queue actual audio cancellation: %d %s", cancelCreate.Code, cancelCreate.Body.String())
	}
	cancelEnv := suite.NewTestWorkflowEnvironment()
	stopped := make(chan struct{})
	cancelEnv.RegisterActivityWithOptions(func(ctx context.Context, id toolapp.WorkID) (tooldomain.ExportJob, error) {
		defer close(stopped)
		return activities.RenderExport(ctx, id)
	}, activity.RegisterOptions{Name: toolflow.RenderActivity})
	cancelEnv.RegisterActivityWithOptions(activities.FailExportWorkflow, activity.RegisterOptions{Name: toolflow.FailureActivity})
	cancelEnv.RegisterDelayedCallback(func() {
		current, err := store.Get(t.Context(), actor, project, cancelJob.ID)
		if err != nil || current.Status != tooldomain.Running || current.Progress < 20 {
			t.Fatalf("actual audio renderer was not running at cancellation: %+v %v", current, err)
		}
		// Progress commits can legitimately advance the revision between GET and
		// POST. Retry the user control against current facts without weakening
		// the production compare-and-swap contract.
		accepted := false
		for range 8 {
			cancelled := request(http.MethodPost, "/api/media-exports/"+cancelJob.ID.String()+"/cancel", toolapp.ControlInput{ProjectID: project, Revision: current.Revision})
			if cancelled.Code == http.StatusAccepted {
				accepted = true
				break
			}
			if cancelled.Code != http.StatusConflict {
				t.Fatalf("actual audio cancel request: %d %s", cancelled.Code, cancelled.Body.String())
			}
			current, err = store.Get(t.Context(), actor, project, cancelJob.ID)
			if err != nil || current.Status != tooldomain.Running {
				t.Fatalf("refresh running cancellation facts: %+v %v", current, err)
			}
		}
		if !accepted {
			t.Fatal("actual audio cancellation did not acquire the current revision")
		}
		cancelEnv.SignalWorkflow("cancel", nil)
	}, 500*time.Millisecond)
	cancelEnv.ExecuteWorkflow(toolflow.ExportWorkflow, toolapp.WorkID{JobID: cancelJob.ID, Attempt: 1})
	if cancelEnv.GetWorkflowError() == nil {
		t.Fatal("actual audio cancellation completed as success")
	}
	// A Workflow future is not cessation evidence. Wait for the production
	// Activity to finish all FFmpeg processes, cleanup and fenced release.
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("actual audio renderer did not stop")
	}
	final, err := store.Get(t.Context(), actor, project, cancelJob.ID)
	if err != nil || final.Status != tooldomain.Cancelled || final.AssetID != nil {
		t.Fatalf("actual audio cancellation facts: %+v %v", final, err)
	}
	var active bool
	if err := db.Raw(`SELECT active_worker IS NOT NULL FROM mediatool.export_job WHERE id=?`, cancelJob.ID).Scan(&active).Error; err != nil || active {
		t.Fatalf("stopped audio retained worker ownership: %v %v", active, err)
	}
}
