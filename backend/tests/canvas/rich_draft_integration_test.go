package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestEditorCanvasUsesFiniteTextOperationsAndOneDraftOwner(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime)
	service := canvasfixture.NewService(store)
	doc, err := service.Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "Rich editor"})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := prompt.DecodeDraft([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"original"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	node := draftNode(t, "image_generation")
	node.Config.Draft = &domain.Draft{Composer: &draft}
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	// label allocation is derived by typed reference operations, never an arbitrary config replacement.
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{canvasfixture.Field(node.ID, []string{"config", "draft", "composer", "labelState"}, map[string]any{})}}); err == nil {
		t.Fatal("arbitrary metadata replacement accepted")
	}
	// Replace only the text atom, retaining paragraph structure and editor metadata.
	atoms, err := domain.PromptSequence(draft.Document)
	if err != nil {
		t.Fatal(err)
	}
	edit := domain.SequenceEdit{{Delete: 8}, {Insert: []domain.Atom{{Type: "text", Text: "edited"}}}}
	if domain.SequenceLength(atoms) != 8 {
		t.Fatal("missing paragraph sequence")
	}
	updated, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "text", NodeID: node.ID, Path: []string{"config", "draft", "composer", "document"}, Text: edit}}})
	if err != nil || updated.Revision != 3 || updated.Nodes[0].InputRevision != 2 {
		t.Fatal(updated, err)
	}
	var versions, independent int
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, node.ID).Scan(&versions).Error; err != nil || versions != 2 {
		t.Fatal(versions, err)
	}
	if err := runtime.Raw(`SELECT count(*) FROM workspace.prompt_draft WHERE project_id=?`, p).Scan(&independent).Error; err != nil || independent != 0 {
		t.Fatal(independent, err)
	}
	body, _ := json.Marshal(updated.Nodes[0].Config.Draft.Composer.Document)
	if string(body) == "null" {
		t.Fatal("editor content lost")
	}
}
