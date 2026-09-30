package canvas_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func resourceNode(kind string) domain.Node {
	n := domain.Node{ID: uuid.New(), Title: "节点", NodeType: kind, NodeAction: "resource", Width: ptr(float64(160)), Height: ptr(float64(100))}
	switch kind {
	case "text":
		n.Config = domain.NodeConfig{Text: ptr("文字")}
	case "group":
		n.Config = domain.NodeConfig{Collapsed: ptr(false)}
	case "image", "video", "audio":
		n.RefType = "media_asset"
		n.RefID = ptr(uuid.New())
	}
	return n
}
func ptr[T any](v T) *T { return &v }

func TestResourceCommandsKeepWorldCoordinatesAndUnbindGroup(t *testing.T) {
	group, a, b := resourceNode("group"), resourceNode("text"), resourceNode("image")
	a.X, a.Y = 100, 200
	b.X, b.Y = 300, 400
	doc := domain.Document{ID: uuid.New(), Revision: 1}
	result, err := domain.Apply(doc, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{group, a, b}}, {Type: "SetNodeParents", Parents: []domain.Parent{{ID: a.ID, ParentID: &group.ID}, {ID: b.ID, ParentID: &group.ID}}}, {Type: "ResizeNodes", Sizes: []domain.Size{{ID: group.ID, Width: 5000, Height: 6000}}}, {Type: "RenameNodes", Names: []domain.Name{{ID: a.ID, Title: "已更名"}}}, {Type: "SetNodeZIndex", ZIndices: []domain.ZIndex{{ID: b.ID, ZIndex: 3}}}, {Type: "Connect", Edges: []domain.Edge{{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: a.ID, TargetNodeID: b.ID}}}, {Type: "SetViewport", Viewport: &domain.Viewport{Zoom: 0.05}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err = domain.Apply(result, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{group.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 || result.Nodes[0].ParentID != nil || result.Nodes[0].X != 100 || result.Nodes[1].RefID == nil || len(result.Edges) != 1 {
		t.Fatalf("group removal changed children: %+v", result)
	}
	if len(doc.Nodes) != 0 {
		t.Fatal("input mutated")
	}
}

func TestResourceCommandsRejectCyclesAndProtectedFieldsAtomically(t *testing.T) {
	a, b := resourceNode("group"), resourceNode("group")
	doc := domain.Document{Nodes: []domain.Node{a, b}}
	_, err := domain.Apply(doc, []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: a.ID, X: 20, Y: 30}}}, {Type: "SetNodeParents", Parents: []domain.Parent{{ID: a.ID, ParentID: &b.ID}, {ID: b.ID, ParentID: &a.ID}}}})
	var indexed *domain.CommandError
	if !errors.Is(err, domain.ErrInvalidCommand) || !errors.As(err, &indexed) || indexed.Index != 1 || doc.Nodes[0].X != 0 {
		t.Fatalf("cycle result %v %+v", err, doc)
	}
	image := resourceNode("image")
	image.Config = domain.NodeConfig{Text: ptr("cannot insert provider input")}
	_, err = domain.Apply(doc, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image}}})
	if !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("media config accepted: %v", err)
	}
}

func TestResourceCommandsBatchAddAndConnectBeyondOneHundredItems(t *testing.T) {
	nodes := make([]domain.Node, 151)
	for i := range nodes {
		nodes[i] = resourceNode("text")
	}
	edges := make([]domain.Edge, 150)
	for i := range edges {
		edges[i] = domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: nodes[i].ID, TargetNodeID: nodes[i+1].ID}
	}
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: nodes}, {Type: "Connect", Edges: edges}, {Type: "Disconnect", IDs: []uuid.UUID{edges[0].ID}}})
	if err != nil || len(doc.Nodes) != 151 || len(doc.Edges) != 149 {
		t.Fatalf("large reversible batch %v", err)
	}
}
