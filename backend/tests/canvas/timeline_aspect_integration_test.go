package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestTimelineAspectPostgresYjsRoundTripAndUnknownRollback(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime)
	writer := canvasfixture.NewEditService(store)
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Timeline ratios"})
	if err != nil {
		t.Fatal(err)
	}
	node := timelineNode()
	if _, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	for _, ratio := range timelineAspectRatios {
		t.Run(ratio, func(t *testing.T) {
			value, _ := json.Marshal(ratio)
			receipt, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "timeline", "aspect_ratio"}, Value: value}}})
			if err != nil || receipt.Status != "accepted" {
				t.Fatal("ratio persistence rejected", receipt, err)
			}
			reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
			if err != nil || reopened.Nodes[0].Config.Timeline.AspectRatio != ratio {
				t.Fatal("ratio lost on reopen", reopened, err)
			}
		})
	}
	before, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ratio := range []string{"future", "2:1", "AUTO", ""} {
		value, _ := json.Marshal(ratio)
		receipt, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "timeline", "aspect_ratio"}, Value: value}}})
		if err != nil || receipt.Status != "rejected" {
			t.Fatal("unsupported ratio changed authoritative graph", receipt, err)
		}
	}
	after, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || after.Revision != before.Revision || after.Nodes[0].Config.Timeline.AspectRatio != "9:16" {
		t.Fatal("rejected ratio leaked", after, err)
	}
}
