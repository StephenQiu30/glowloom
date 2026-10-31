package canvas_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCanvasContainerPostgresNestedTreeAndReopen(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("restricted runtime required", role, err)
	}
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Containers"})
	if err != nil {
		t.Fatal(err)
	}
	outer := domain.Node{ID: uuid.New(), NodeType: "group", NodeAction: "resource", Title: "普通组", Config: domain.NodeConfig{Collapsed: ptr(false), Container: &domain.ContainerConfig{Version: 1, Kind: "group", Background: "cyan"}}}
	stack := domain.Node{ID: uuid.New(), NodeType: "group", NodeAction: "resource", Title: "堆叠", ParentID: &outer.ID, X: 200, Y: 300, Config: domain.NodeConfig{Collapsed: ptr(false), Container: &domain.ContainerConfig{Version: 1, Kind: "stack", Background: "none"}}}
	child := domain.Node{ID: uuid.New(), NodeType: "text", NodeAction: "resource", Title: "原文本", ParentID: &stack.ID, X: 600, Y: 700, Config: domain.NodeConfig{Text: ptr("原始内容")}}
	if _, err = writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{outer, stack, child})}); err != nil {
		t.Fatal(err)
	}
	reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[uuid.UUID]domain.Node{}
	for _, n := range reopened.Nodes {
		byID[n.ID] = n
	}
	body, _ := json.Marshal(byID[child.ID].Config)
	if len(byID) != 3 || byID[stack.ID].Config.Container == nil || byID[outer.ID].Config.Container == nil || byID[child.ID].ParentID == nil || byID[stack.ID].Config.Container.Kind != "stack" || byID[outer.ID].Config.Container.Background != "cyan" || *byID[child.ID].ParentID != stack.ID || byID[child.ID].X != 600 || !strings.Contains(string(body), "原始内容") {
		t.Fatalf("lost container tree %+v", reopened.Nodes)
	}
}
