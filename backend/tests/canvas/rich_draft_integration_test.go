package canvas_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestEditorCanvasMetadataUsesOneOwnerAndKeepsInputRevision(t *testing.T) {
	owner := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "LV_TEST_CANVAS_M1_RUNTIME_DSN")
	service := application.NewService(canvasStore(runtime))
	doc, err := service.Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "Rich editor"})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := promptdomain.DecodeDraft([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"original"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	node := draftNode(t, "image_generation")
	node.Config.Draft = &domain.Draft{Composer: &draft}
	added, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}})
	if err != nil {
		t.Fatal(err)
	}
	draft.LabelState.HighWater["image"] = 1
	draft.LabelState.Allocations["removed"] = promptdomain.LabelAllocation{LabelKind: "image", Ordinal: 1}
	config := domain.NodeConfig{Draft: &domain.Draft{Composer: &draft}}
	changed, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: added.Revision, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &config}}})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision != 3 || changed.Nodes[0].InputRevision != 1 || changed.Nodes[0].Config.Draft.Composer.LabelState.HighWater["image"] != 1 {
		t.Fatal("metadata revision", changed)
	}
	var versions int
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, node.ID).Scan(&versions).Error; err != nil || versions != 1 {
		t.Fatal("metadata created input version", versions, err)
	}
	draft.Document.Content[0].Content[0].Text = "edited"
	updated, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &config}}})
	if err != nil || updated.Nodes[0].InputRevision != 2 {
		t.Fatal("input revision", err)
	}
	legacy := draftNode(t, "image_generation").Config
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 4, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &legacy}}}); err == nil {
		t.Fatal("legacy overwrite accepted")
	}
	var independent int
	if err := runtime.Raw(`SELECT count(*) FROM workspace.prompt_draft WHERE project_id=?`, p).Scan(&independent).Error; err != nil || independent != 0 {
		t.Fatal("second owner", independent, err)
	}
}
