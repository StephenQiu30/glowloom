package canvas_test

import (
	"encoding/json"
	"testing"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCanvasContainerTypedKindsAndClosedConfig(t *testing.T) {
	base := domain.Node{ID: uuid.New(), NodeType: "group", NodeAction: "resource", Title: "Container", Config: domain.NodeConfig{Collapsed: ptr(false), Container: &domain.ContainerConfig{Version: 1, Kind: "stack", Background: "none"}}}
	for _, kind := range []string{"group", "stack"} {
		n := base
		n.Config.Container = &domain.ContainerConfig{Version: 1, Kind: kind, Background: "cyan"}
		out, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}})
		if err != nil || out.Nodes[0].Config.Container.Kind != kind {
			t.Fatalf("container lost %+v %v", out, err)
		}
	}
	for _, value := range []domain.ContainerConfig{{Version: 2, Kind: "stack", Background: "none"}, {Version: 1, Kind: "storyboard", Background: "none"}, {Version: 1, Kind: "group", Background: "url(external)"}} {
		n := base
		n.Config.Container = &value
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}}); err == nil {
			t.Fatalf("invalid accepted %+v", value)
		}
	}
	n := base
	n.NodeType = "text"
	n.Config.Text = ptr("text")
	n.Config.Collapsed = nil
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}}); err == nil {
		t.Fatal("container on non-group accepted")
	}
}

func TestCanvasContainerYjsProjectionUpdatesAndRejectsInvalidKind(t *testing.T) {
	n := domain.Node{ID: uuid.New(), NodeType: "group", NodeAction: "resource", Title: "Group", Config: domain.NodeConfig{Collapsed: ptr(false), Container: &domain.ContainerConfig{Version: 1, Kind: "stack", Background: "none"}}}
	before := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "Container", SchemaVersion: 1, Revision: 2, Nodes: []domain.Node{n}, Edges: []domain.Edge{}}
	projected := canvas.Snapshot(before)
	body, _ := json.Marshal(projected)
	_ = json.Unmarshal(body, &projected)
	projected.Nodes[0].Config.Container.Background = "purple"
	ops, err := canvas.YjsProjectionOperations(before, projected)
	if err != nil {
		t.Fatal(err)
	}
	after, err := domain.ApplyOperations(before, ops)
	if err != nil || after.Document.Nodes[0].Config.Container.Background != "purple" {
		t.Fatalf("container projection dropped update %+v %v", after, err)
	}
	projected.Nodes[0].Config.Container.Kind = "storyboard"
	if _, err = canvas.YjsProjectionOperations(before, projected); err == nil {
		t.Fatal("unknown container accepted")
	}
}
