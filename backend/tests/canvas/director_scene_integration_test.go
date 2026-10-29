package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestDirectorScenePostgresControlsFramesAndReopen(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("must use restricted runtime role", role, err)
	}
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Director scene controls"})
	if err != nil {
		t.Fatal(err)
	}
	node := directorNode()
	node.Config.Director.Mode = "dynamic"
	node.Config.Director.Objects[0].Locked = ptr(true)
	node.Config.Director.Ground = &domain.DirectorGround{Visible: true, Opacity: 1, Color: "#242424"}
	node.Config.Director.Shots[0].FPS = 60
	node.Config.Director.Cameras[0].Keyframes = []domain.DirectorKeyframe{{ID: uuid.New(), Time: 2, Transform: domain.DirectorTransform{Position: []float64{5, 3, 7}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}, Easing: "smooth"}}
	if _, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Nodes[0].Config.Director
	if got.Mode != "dynamic" || !*got.Objects[0].Locked || got.Ground.Color != "#242424" || got.Shots[0].FPS != 60 || got.Cameras[0].Keyframes[0].Transform.Position[0] != 5 {
		t.Fatalf("scene changed on reopen: %+v", got)
	}
	before := reopened.Revision
	value, _ := json.Marshal("unknown_mode")
	receipt, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "director", "mode"}, Value: value}}})
	if err != nil || receipt.Status != "rejected" {
		t.Fatal("invalid mode was not rejected", receipt, err)
	}
	after, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || after.Revision != before || after.Nodes[0].Config.Director.Mode != "dynamic" {
		t.Fatal("invalid mode leaked into stored scene", after, err)
	}
}
