package canvas_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
	"github.com/google/uuid"
)

func TestTimelineRelationsPersistAtomicallyAndRejectStaleMediaDuration(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "时间线领域原子连接"})
	if err != nil {
		t.Fatal(err)
	}
	image, video := resourceNode("image"), resourceNode("video")
	imageAsset, videoAsset := seedCanvasMedia(t, owner, project, "image"), seedCanvasMedia(t, owner, project, "video")
	if err := owner.Exec(`UPDATE media.media_asset SET duration_ms=7200 WHERE id=?`, videoAsset).Error; err != nil {
		t.Fatal(err)
	}
	image.RefID, video.RefID = &imageAsset, &videoAsset
	target := timelineNode()
	target.Config.Timeline.Tracks[0].Kind = "video"
	trackID := target.Config.Timeline.Tracks[0].ID
	duration := int64(7200)
	target.Config.Timeline.Clips = []domain.TimelineClip{
		{ID: uuid.New(), TrackID: trackID, Kind: "image", NodeID: &image.ID, AssetID: &imageAsset, Title: "image", DurationMS: 5000, Volume: 1},
		{ID: uuid.New(), TrackID: trackID, Kind: "video", NodeID: &video.ID, AssetID: &videoAsset, Title: "video", StartMS: 5000, DurationMS: 7200, SourceDurationMS: &duration, Volume: 1},
	}
	edges := []domain.Edge{
		{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: image.ID, TargetNodeID: target.ID},
		{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: video.ID, TargetNodeID: target.ID},
	}
	operations := canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{image, video, target}), canvasfixture.Edges(edges))
	// A metadata drift must reject the entire graph, including the otherwise valid image input.
	if err := owner.Exec(`UPDATE media.media_asset SET duration_ms=7000 WHERE id=?`, videoAsset).Error; err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: operations})
	if err != nil || receipt.Status != "rejected" {
		t.Fatal("stale duration admitted", receipt, err)
	}
	after, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || after.Revision != doc.Revision || len(after.Nodes) != 0 || len(after.Edges) != 0 {
		t.Fatal("rejected batch partially persisted", after, err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET duration_ms=7200 WHERE id=?`, videoAsset).Error; err != nil {
		t.Fatal(err)
	}
	receipt, err = writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: operations})
	if err != nil || receipt.Status != "accepted" {
		t.Fatal("valid complete batch rejected", receipt, err)
	}
	reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(reopened.Nodes) != 3 || len(reopened.Edges) != 2 {
		t.Fatal("relations lost after reopen", reopened, err)
	}
	for _, node := range reopened.Nodes {
		if node.ID == target.ID && (len(node.Config.Timeline.Clips) != 2 || node.Config.Timeline.Clips[0].NodeID == nil || node.Config.Timeline.Clips[1].NodeID == nil) {
			t.Fatal("clip bindings lost after reopen")
		}
	}
	// A bare disconnect must fail without leaving a live clip bound to an absent edge.
	receipt, err = writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: []domain.Operation{{Type: "disconnect", NodeID: edges[0].ID}}})
	if err != nil || receipt.Status != "rejected" {
		t.Fatal("non-atomic disconnect admitted", receipt, err)
	}
	unchanged, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || unchanged.Revision != reopened.Revision || len(unchanged.Edges) != 2 {
		t.Fatal("rejected disconnect changed graph", unchanged, err)
	}
}
