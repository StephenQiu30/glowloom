package canvas_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestTimelineImportedShelfPersistsWithoutClipsAndRejectsForeignAssets(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := canvasfixture.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "时间线导入资产"})
	if err != nil {
		t.Fatal(err)
	}
	assets := []uuid.UUID{seedCanvasMedia(t, database, project, "video"), seedCanvasMedia(t, database, project, "image"), seedCanvasMedia(t, database, project, "audio")}
	node := timelineNode()
	node.Config.Timeline.Tracks[0].Kind = "video"
	node.Config.Timeline.Clips = nil
	node.Config.Timeline.ImportedAssets = assets
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal("independent imports rejected", err)
	}
	reopened, err := application.NewService(canvasStore(database)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(reopened.Nodes) != 1 || len(reopened.Nodes[0].Config.Timeline.ImportedAssets) != 3 || len(reopened.Nodes[0].Config.Timeline.Clips) != 0 {
		t.Fatal("import shelf did not survive reopen without clips", err)
	}
	for _, asset := range assets {
		found, err := canvasReferenceCheck(t, database, actor, project, asset)
		if err != nil || !found {
			t.Fatal("unplaced import is not protected from deletion", asset, err)
		}
	}
	otherProject := uuid.New()
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'其他项目','16:9','realistic')`, otherProject, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	_, foreignProject := seedCanvasActor(t, database)
	for _, asset := range []uuid.UUID{seedCanvasMedia(t, database, otherProject, "image"), seedCanvasMedia(t, database, foreignProject, "image"), uuid.New()} {
		foreignNode := timelineNode()
		foreignNode.Config.Timeline.Clips = nil
		foreignNode.Config.Timeline.ImportedAssets = []uuid.UUID{asset}
		if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{foreignNode})}); err == nil {
			t.Fatal("foreign or missing imported asset accepted")
		}
	}
	after, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || after.Revision != reopened.Revision || len(after.Nodes) != 1 {
		t.Fatal("failed imported asset admission changed the document", err)
	}
}
