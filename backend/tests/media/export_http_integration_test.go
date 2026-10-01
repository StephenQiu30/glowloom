package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	toolhttp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/http"
	toolpg "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestExportRealHTTPFrozenCanvasPrivateRenderingReviewAndDownload(t *testing.T) {
	db := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	_, foreign := mediaStoreProject(t, db)
	var imageBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 32 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	if err := png.Encode(&imageBytes, img); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(t.TempDir(), "input-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(imageBytes.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(imageBytes.Bytes())
	download := &mediaapp.Downloaded{File: file, Size: int64(imageBytes.Len()), MIMEType: "image/png", SHA256: hex.EncodeToString(hash[:])}
	defer func() { _ = download.Close() }()
	mediaStore := mediapg.NewStore(db)
	upload := mediaapp.NewUploadService(mediaStore, mediaflow.FFUploadProber{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	uploaded, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "source.png"}, File: download, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup only this test's exact private keys, including partial artifacts.
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
				t.Error("clean exact test export object")
			}
		}
	})
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }
	canvasStore := canvaspg.NewStore(db, mediaFactory)
	canvasService := canvasapp.NewService(canvasStore)
	doc, err := canvasService.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "Export real fixture"})
	if err != nil {
		t.Fatal(err)
	}
	config := exportTimeline(t)
	config.Clips[0].AssetID = &uploaded.Asset.ID
	config.Clips[0].Crop = &canvasdomain.TimelineCrop{X: 32, Y: 0, Width: 32, Height: 64}
	lane := uuid.New()
	config.Tracks = append(config.Tracks, canvasdomain.TimelineTrack{ID: lane, Kind: "subtitle", Label: "Subtitle", Visible: true})
	config.Clips = append(config.Clips, canvasdomain.TimelineClip{ID: uuid.New(), TrackID: lane, Kind: "subtitle", Title: "Subtitle", Text: "中文 AI\n实际导出", DurationMS: 200, Volume: 1})
	node := canvasdomain.Node{ID: uuid.New(), Title: "Timeline", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &config}}
	saved, err := canvasService.Execute(t.Context(), actor, doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: doc.Revision, Commands: []canvasdomain.Command{{Type: "AddNodes", Nodes: []canvasdomain.Node{node}}}})
	if err != nil {
		t.Fatal(err)
	}
	store := toolpg.NewStore(db, func(tx *gorm.DB) toolapp.TimelineReader {
		return canvasapp.NewTimelineReader(canvaspg.NewStore(tx, mediaFactory))
	}, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediapg.NewStore(tx)) })
	query := toolapp.NewExportQuery(store, objects, objects)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	toolhttp.NewHandler(store, query).Register(g)
	request := func(method, url string, key uuid.UUID, body any) *httptest.ResponseRecorder {
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
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Content-Type", "application/json")
		if key != uuid.Nil {
			req.Header.Set("Idempotency-Key", key.String())
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	createURL := "/api/projects/" + project.String() + "/media-exports"
	key := uuid.New()
	input := toolapp.CreateInput{CanvasID: doc.ID, NodeID: node.ID, Revision: saved.Revision}
	accepted := request("POST", createURL, key, input)
	if accepted.Code != 202 {
		t.Fatalf("create %d: %s", accepted.Code, accepted.Body.String())
	}
	var job tooldomain.ExportJob
	if err := json.Unmarshal(accepted.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	same := request("POST", createURL, key, input)
	if same.Code != 202 || same.Body.String() != accepted.Body.String() {
		t.Fatal("create response was not a durable replay")
	}
	stale := input
	stale.Revision--
	if got := request("POST", createURL, uuid.New(), stale); got.Code != 409 {
		t.Fatalf("stale source accepted %d", got.Code)
	}
	jobURL := "/api/media-exports/" + job.ID.String()
	scope := "?project_id=" + project.String()
	if got := request("GET", jobURL+"?project_id="+foreign.String(), uuid.Nil, nil); got.Code != 404 {
		t.Fatalf("foreign job %d", got.Code)
	}
	renderer, err := toolff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	worker := toolapp.NewWorker(store, toolflow.NewObjects(objects), renderer, mediaflow.FFProber{}, mediaflow.FFRenderer{})
	var suite testsuite.WorkflowTestSuite
	workflowEnv := suite.NewTestWorkflowEnvironment()
	activities := toolflow.NewActivities(worker, store, store)
	workflowEnv.RegisterWorkflow(toolflow.ExportWorkflow)
	workflowEnv.RegisterActivityWithOptions(activities.RenderExport, activity.RegisterOptions{Name: toolflow.RenderActivity})
	workflowEnv.RegisterActivityWithOptions(activities.FailExportWorkflow, activity.RegisterOptions{Name: toolflow.FailureActivity})
	workflowEnv.ExecuteWorkflow(toolflow.ExportWorkflow, toolapp.WorkID{JobID: job.ID, Attempt: job.Attempt})
	if !workflowEnv.IsWorkflowCompleted() || workflowEnv.GetWorkflowError() != nil {
		t.Fatalf("real render under SDK orchestration: %v", workflowEnv.GetWorkflowError())
	}
	var rendered tooldomain.ExportJob
	if err := workflowEnv.GetWorkflowResult(&rendered); err != nil || rendered.Status != tooldomain.ReviewRequired || rendered.AssetID == nil || rendered.SHA256 == nil {
		t.Fatalf("real render %+v %v", rendered, err)
	}
	if _, err := mediaapp.NewAssetQuery(mediaStore, objects).Reference(t.Context(), actor, project, *rendered.AssetID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("unreviewed output referenceable %v", err)
	}
	before := request("GET", jobURL+"/download"+scope, uuid.Nil, nil)
	if before.Code != 409 {
		t.Fatalf("unreviewed download %d", before.Code)
	}
	preview := request("GET", jobURL+"/preview"+scope, uuid.Nil, nil)
	if preview.Code != 200 {
		t.Fatalf("preview %d: %s", preview.Code, preview.Body.String())
	}
	var view toolapp.ExportPreview
	if json.Unmarshal(preview.Body.Bytes(), &view) != nil || view.SHA256 != *rendered.SHA256 || view.Revision != rendered.Revision || view.Asset.Kind != "video" || view.Asset.Width == nil || *view.Asset.Width != 1080 {
		t.Fatal("actual output preview facts")
	}
	fetchRequest, err := http.NewRequestWithContext(t.Context(), "GET", view.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	fetched, err := http.DefaultClient.Do(fetchRequest)
	if err != nil {
		t.Fatal("fetch actual private export")
	}
	data, err := io.ReadAll(io.LimitReader(fetched.Body, view.Asset.ByteSize+1))
	_ = fetched.Body.Close()
	sum := sha256.Sum256(data)
	if err != nil || fetched.StatusCode != 200 || hex.EncodeToString(sum[:]) != view.SHA256 {
		t.Fatal("private MP4 differs from review hash")
	}
	inspected, err := os.CreateTemp(t.TempDir(), "inspect-*.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inspected.Write(data); err != nil {
		t.Fatal(err)
	}
	_ = inspected.Close()
	assertBlueCrop(t, inspected.Name())
	review := toolapp.ReviewInput{ProjectID: project, Revision: view.Revision, SHA256: view.SHA256, LocalReviewConfirmed: false}
	if got := request("POST", jobURL+"/review", uuid.New(), review); got.Code != 422 {
		t.Fatalf("missing actual review %d", got.Code)
	}
	review.LocalReviewConfirmed = true
	bad := review
	bad.SHA256 = strings.Repeat("0", 64)
	if got := request("POST", jobURL+"/review", uuid.New(), bad); got.Code != 409 {
		t.Fatalf("wrong inspected SHA %d", got.Code)
	}
	reviewKey := uuid.New()
	approved := request("POST", jobURL+"/review", reviewKey, review)
	if approved.Code != 200 {
		t.Fatalf("approve %d: %s", approved.Code, approved.Body.String())
	}
	var final tooldomain.ExportJob
	if json.Unmarshal(approved.Body.Bytes(), &final) != nil || final.Status != tooldomain.Succeeded || final.Progress != 100 {
		t.Fatal("final reviewed job facts")
	}
	if got := request("POST", jobURL+"/review", reviewKey, review); got.Code != 200 || got.Body.String() != approved.Body.String() {
		t.Fatal("review replay")
	}
	attachment := request("GET", jobURL+"/download"+scope, uuid.Nil, nil)
	downloadHash := sha256.Sum256(attachment.Body.Bytes())
	if attachment.Code != 200 || !strings.HasPrefix(attachment.Header().Get("Content-Disposition"), "attachment;") || hex.EncodeToString(downloadHash[:]) != view.SHA256 {
		t.Fatal("actual reviewed attachment download")
	}
	srt := request("GET", jobURL+"/subtitles"+scope, uuid.Nil, nil)
	if srt.Code != 200 || !strings.Contains(srt.Body.String(), "00:00:00,000 --> 00:00:00,200") || !strings.Contains(srt.Body.String(), "中文 AI\n实际导出") {
		t.Fatal("actual frozen SRT")
	}
	list := request("GET", createURL+"?canvas_id="+doc.ID.String()+"&node_id="+node.ID.String(), uuid.Nil, nil)
	var page toolapp.ExportPage
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].Status != tooldomain.Succeeded {
		t.Fatalf("restored durable jobs %d %s", list.Code, list.Body.String())
	}
	repeat, err := worker.Execute(t.Context(), toolapp.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || repeat.ID != final.ID || repeat.Revision != final.Revision {
		t.Fatal("completed activity retry rerendered")
	}
	var events []struct {
		Topic, PartitionKey string
		Payload             []byte
	}
	if err := db.Raw(`SELECT topic,partition_key,payload FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->>'project_id'=? AND payload->'data'->'object'->>'type'='media_export' ORDER BY create_time,id`, project.String()).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("actual produced export audit events count=%d err=%v", len(events), err)
	}
	auditConsumer := auditevent.NewHandler(inboxpg.NewStore(db), auditapp.NewRecordedActionParser())
	for _, event := range events {
		record := inboxapp.Record{Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload}
		for range 2 {
			if err := auditConsumer.Handle(t.Context(), record); err != nil {
				t.Fatalf("produced audit event violates strict scalar policy: %v", err)
			}
		}
	}
	var counts struct{ Assets, Commands, Outboxes, Audits int }
	if err := db.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?) assets,(SELECT count(*) FROM mediatool.export_command WHERE job_id=?) commands,(SELECT count(*) FROM infra.outbox WHERE topic='lanverse.mediatool.export_command.v1' AND payload->'data'->>'job_id'=?) outboxes,(SELECT count(*) FROM audit.audit_log WHERE project_id=? AND object_type='media_export') audits`, project, job.ID, job.ID.String(), project).Scan(&counts).Error; err != nil || counts.Assets != 2 || counts.Commands != 2 || counts.Outboxes != 1 || counts.Audits != 2 {
		t.Fatalf("durable export facts=%+v err=%v", counts, err)
	}
	// A real renderer activity must return and relinquish its own worker session
	// before the durable cancellation becomes terminal.
	cancelling, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	cancelEnv := suite.NewTestWorkflowEnvironment()
	cancelEnv.RegisterWorkflow(toolflow.ExportWorkflow)
	stoppedActivity := make(chan struct{})
	cancelEnv.RegisterActivityWithOptions(func(ctx context.Context, id toolapp.WorkID) (tooldomain.ExportJob, error) {
		defer close(stoppedActivity)
		return activities.RenderExport(ctx, id)
	}, activity.RegisterOptions{Name: toolflow.RenderActivity})
	cancelEnv.RegisterActivityWithOptions(activities.FailExportWorkflow, activity.RegisterOptions{Name: toolflow.FailureActivity})
	callback := make(chan error, 1)
	cancelEnv.RegisterDelayedCallback(func() {
		current, err := store.Get(t.Context(), actor, project, cancelling.ID)
		if err == nil && current.Status != tooldomain.Running {
			err = fmt.Errorf("expected actual running renderer, got %s", current.Status)
		}
		if err == nil {
			_, err = store.Control(t.Context(), actor, project, current.ID, uuid.New(), current.Revision, "cancel")
		}
		callback <- err
		cancelEnv.SignalWorkflow("cancel", nil)
	}, 100*time.Millisecond)
	cancelEnv.ExecuteWorkflow(toolflow.ExportWorkflow, toolapp.WorkID{JobID: cancelling.ID, Attempt: 1})
	if err := <-callback; err != nil {
		t.Fatal(err)
	}
	if !cancelEnv.IsWorkflowCompleted() || cancelEnv.GetWorkflowError() == nil {
		t.Fatal("renderer cancellation did not finish the cancelled workflow")
	}
	select {
	case <-stoppedActivity:
	case <-time.After(5 * time.Second):
		t.Fatal("actual export activity failed to exit after cancellation")
	}
	stopped, err := store.Get(t.Context(), actor, project, cancelling.ID)
	if err != nil || stopped.Status != tooldomain.Cancelled || stopped.AssetID != nil {
		t.Fatalf("actual renderer cessation receipt: %+v %v", stopped, err)
	}
	var active int
	if err := db.Raw(`SELECT count(*) FROM mediatool.export_job WHERE id=? AND active_worker IS NOT NULL`, cancelling.ID).Scan(&active).Error; err != nil || active != 0 {
		t.Fatal("cancelled renderer retained an active execution")
	}
}

func assertBlueCrop(t *testing.T, file string) {
	t.Helper()
	raw, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-i", file, "-frames:v", "1", "-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := frame.At(540, 400).RGBA()
	if b < 40000 || r > 20000 || g > 20000 {
		t.Fatalf("actual crop did not select blue right source pixels: %d %d %d", r, g, b)
	}
}
