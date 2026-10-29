package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	toolhttp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/http"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestTimelineExportSettingsDurableReplayAndAutomaticPublication(t *testing.T) {
	db := mediaStoreDB(t)
	objects := glbTestObjects(t)
	owner := db
	if os.Getenv("GLOWLOOM_TEST_PROJECT_EXPORT_OWNER_DSN") != "" {
		owner = projectOwnerDB(t)
	}
	actor, project := mediaStoreProject(t, owner)
	timeline := exportTimeline(t)
	timeline.AspectRatio = "auto"
	var source bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 64, 36))
	for y := range 36 {
		for x := range 64 {
			picture.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	if err := png.Encode(&source, picture); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(t.TempDir(), "source-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(source.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	sha := sha256.Sum256(source.Bytes())
	download := &mediaapp.Downloaded{File: file, Size: int64(source.Len()), MIMEType: "image/png", SHA256: hex.EncodeToString(sha[:])}
	defer download.Close()
	upload := mediaapp.NewUploadService(mediaTestStore(db), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	uploaded, err := upload.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "synthetic.png"}, File: download, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	timeline.Tracks[0].Kind = "video"
	timeline.Clips[0].Kind = "image"
	timeline.Clips[0].AssetID = &uploaded.Asset.ID
	timeline.ImportedAssets = []uuid.UUID{uploaded.Asset.ID}
	canvasService := canvasfixture.NewService(mediaCanvasStore(db, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediaTestStore(tx), nil) }))
	doc, err := canvasService.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "时间线设置"})
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), Title: "时间线设置", NodeType: "timeline", NodeAction: "tool", Config: canvasdomain.NodeConfig{Timeline: &timeline}}
	saved, err := canvasService.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: doc.Revision, Operations: canvasfixture.Nodes([]canvasdomain.Node{node})})
	if err != nil {
		t.Fatal(err)
	}
	store := mediawire.ProvideMediaExportStore(db)
	workerStore := mediawire.ProvideMediaExportStoreWithAccess(db, mediawire.NewMediaWorkerScopeAccess)
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
				t.Error("remove isolated export object")
			}
		}
	})
	key := uuid.New()
	input := toolapp.CreateInput{CanvasID: doc.ID, NodeID: node.ID, Revision: saved.Revision, Settings: &tool.ExportSettings{FileName: "时间线验证", Resolution: 480, Destination: "canvas"}}
	router := gin.New()
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	toolhttp.NewHandler(store, toolapp.NewExportQuery(store, objects, objects)).Register(group)
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+project.String()+"/media-exports", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key.String())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var job tool.ExportJob
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &job) != nil || job.Settings == nil || job.Settings.Resolution != 480 {
		t.Fatalf("HTTP frozen settings status=%d", response.Code)
	}
	replay, err := store.Create(t.Context(), actor, project, key, input)
	if err != nil || replay.ID != job.ID {
		t.Fatal("settings receipt replay", err)
	}
	for _, change := range []tool.ExportSettings{{FileName: "变更", Resolution: 480, Destination: "canvas"}, {FileName: "时间线验证", Resolution: 720, Destination: "canvas"}, {FileName: "时间线验证", Resolution: 480, Destination: "local"}} {
		changed := input
		changed.Settings = &change
		if _, err := store.Create(t.Context(), actor, project, key, changed); !errors.Is(err, toolapp.ErrConflict) {
			t.Fatal("changed settings reused receipt", err)
		}
	}
	frozen, err := store.Frozen(t.Context(), actor, project, job.ID)
	if err != nil || frozen.Width != 854 || frozen.Height != 480 || frozen.Timeline.AspectRatio != "auto" {
		t.Fatalf("frozen plan %+v %v", frozen.Settings, err)
	}
	renderer, err := toolff.NewRenderer(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	worker := toolapp.NewWorker(workerStore, toolflow.NewObjects(objects), renderer, mediaflow.FFProber{}, mediaflow.FFRenderer{})
	result, err := worker.Execute(t.Context(), toolapp.WorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err != nil || result.Status != tool.Succeeded || result.AssetID == nil {
		t.Fatalf("automatic publication %+v %v", result, err)
	}
	asset, err := mediaapp.NewAssetQuery(mediaTestStore(db), objects).Reference(t.Context(), actor, project, *result.AssetID)
	if err != nil || asset.FileName != "时间线验证.mp4" || asset.Width == nil || *asset.Width != 854 || asset.Height == nil || *asset.Height != 480 {
		t.Fatalf("referenceable settings output %+v %v", asset, err)
	}
	var method string
	if err := db.Raw(`SELECT moderation_detail->>'method' FROM media.media_asset WHERE id=?`, *result.AssetID).Scan(&method).Error; err != nil || method != "deterministic_timeline_source_validation" {
		t.Fatal("output invented an owner review", err)
	}
	request = httptest.NewRequestWithContext(t.Context(), "GET", "/api/media-exports/"+job.ID.String()+"/download?project_id="+project.String(), nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	_, params, dispositionErr := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
	if response.Code != 200 || response.Body.Len() == 0 || dispositionErr != nil || params["filename"] != "时间线验证.mp4" {
		t.Fatalf("HTTP real download status=%d, filename=%q, error=%v", response.Code, params["filename"], dispositionErr)
	}
}

func TestTimelineExportHistoryRestoresNewestCreatedJob(t *testing.T) {
	db := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, db)
	timeline := exportTimeline(t)
	timeline.Tracks[0].Kind = "subtitle"
	timeline.Clips[0].Kind = "subtitle"
	timeline.Clips[0].AssetID = nil
	timeline.Clips[0].Text = "历史"
	store := toolpg.NewStore(db, func(*gorm.DB) toolapp.TimelineReader { return &fixedTimeline{timeline: timeline} }, func(tx *gorm.DB) toolapp.DerivedMedia { return mediaapp.NewDerivedService(mediaTestStore(tx)) })
	input := toolapp.CreateInput{CanvasID: uuid.New(), NodeID: uuid.New(), Revision: 1, Settings: &tool.ExportSettings{FileName: "历史"}}
	first, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(t.Context(), actor, project, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	newer, older := first, second
	if newer.ID.String() > older.ID.String() {
		newer, older = older, newer
	}
	if err := db.Exec(`UPDATE mediatool.export_job SET created_at=now()-interval '1 hour' WHERE id=?`, older.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE mediatool.export_job SET created_at=now() WHERE id=?`, newer.ID).Error; err != nil {
		t.Fatal(err)
	}
	page, err := store.List(t.Context(), actor, toolapp.ListInput{ProjectID: project, CanvasID: input.CanvasID, NodeID: input.NodeID, Limit: 1})
	if err != nil || len(page) != 1 || page[0].ID != newer.ID {
		t.Fatalf("latest task must sort by creation time, got %+v %v", page, err)
	}
	next, err := store.List(t.Context(), actor, toolapp.ListInput{ProjectID: project, CanvasID: input.CanvasID, NodeID: input.NodeID, After: newer.ID, Limit: 1})
	if err != nil || len(next) != 1 || next[0].ID != older.ID {
		t.Fatalf("history cursor lost older task %+v %v", next, err)
	}
}
