package canvas_test

import (
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCanvasCommandsAreAtomicAndRejectBusinessBindings(t *testing.T) {
	id := uuid.New()
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Revision: 1, Nodes: []domain.Node{}, Edges: []domain.Edge{}}
	added, err := domain.Apply(doc, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{{ID: id, Title: "备注", NodeType: "text", NodeAction: "resource", Config: domain.NodeConfig{Text: ptr("备注")}, X: 10, Y: 20}}}})
	if err != nil || len(added.Nodes) != 1 || len(doc.Nodes) != 0 {
		t.Fatalf("add: %+v, %v", added, err)
	}
	_, err = domain.Apply(added, []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: id, X: 50, Y: 60}}}, {Type: "DeleteNodes", IDs: []uuid.UUID{uuid.New()}}})
	if !errors.Is(err, domain.ErrInvalidCommand) || added.Nodes[0].X != 10 {
		t.Fatalf("partial mutation: %+v %v", added, err)
	}
	_, err = domain.Apply(added, []domain.Command{{Type: "Connect", Edges: []domain.Edge{{ID: uuid.New(), EdgeType: "reference", SourceNodeID: id, TargetNodeID: id}}}})
	if !errors.Is(err, domain.ErrUnsupportedCommand) {
		t.Fatalf("reference allowed: %v", err)
	}
	_, err = domain.Apply(added, []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: id, X: math.Inf(1)}}}})
	if !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("infinite coordinate allowed: %v", err)
	}
	added.Nodes[0].RefType = "shot"
	_, err = domain.Apply(added, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{id}}})
	if !errors.Is(err, domain.ErrUnsupportedCommand) {
		t.Fatalf("business node deleted: %v", err)
	}
}

func TestCanvasAnnotationAndViewport(t *testing.T) {
	a, b, edge := uuid.New(), uuid.New(), uuid.New()
	doc := domain.Document{ID: uuid.New(), Nodes: []domain.Node{
		{ID: a, Title: "备注", NodeType: "text", NodeAction: "resource"}, {ID: b, Title: "备注", NodeType: "text", NodeAction: "resource"},
	}, Edges: []domain.Edge{}}
	updated, err := domain.Apply(doc, []domain.Command{{Type: "Connect", Edges: []domain.Edge{{ID: edge, EdgeType: "annotation", SourceNodeID: a, TargetNodeID: b}}}, {Type: "SetViewport", Viewport: &domain.Viewport{X: 12, Y: 34, Zoom: 1.2}}})
	if err != nil || len(updated.Edges) != 1 || updated.Viewport.Zoom != 1.2 {
		t.Fatalf("apply: %+v %v", updated, err)
	}
	deleted, err := domain.Apply(updated, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{a}}})
	if err != nil || len(deleted.Nodes) != 1 || len(deleted.Edges) != 0 {
		t.Fatalf("delete: %+v %v", deleted, err)
	}
}

func TestCanvasViewportSupportsJimengZoomRange(t *testing.T) {
	doc := domain.Document{ID: uuid.New(), Nodes: []domain.Node{}, Edges: []domain.Edge{}}
	for _, zoom := range []float64{0.08, 5, 8} {
		result, err := domain.Apply(doc, []domain.Command{{Type: "SetViewport", Viewport: &domain.Viewport{Zoom: zoom}}})
		if err != nil || result.Viewport.Zoom != zoom {
			t.Fatalf("zoom %v: got %v, %v", zoom, result.Viewport.Zoom, err)
		}
	}
	for _, zoom := range []float64{8.01, math.Inf(1), math.NaN()} {
		if _, err := domain.Apply(doc, []domain.Command{{Type: "SetViewport", Viewport: &domain.Viewport{Zoom: zoom}}}); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("invalid zoom %v accepted: %v", zoom, err)
		}
	}
}

func TestCanvasDraftLayoutConnectionsDoNotGrantBusinessBindings(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	doc := domain.Document{ID: uuid.New(), Nodes: []domain.Node{
		{ID: a, Title: "图片草稿", NodeType: "image_generation", NodeAction: "generate"},
		{ID: b, Title: "视频草稿", NodeType: "video_generation", NodeAction: "generate"},
	}, Edges: []domain.Edge{}}
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: a, TargetNodeID: b}
	result, err := domain.Apply(doc, []domain.Command{{Type: "Connect", Edges: []domain.Edge{edge}}})
	if err != nil || len(result.Edges) != 1 || len(doc.Edges) != 0 {
		t.Fatalf("draft layout connection: %+v %v", result, err)
	}
	edge.Role = "reference_image"
	if _, err := domain.Apply(doc, []domain.Command{{Type: "Connect", Edges: []domain.Edge{edge}}}); !errors.Is(err, domain.ErrUnsupportedCommand) {
		t.Fatalf("layout connection accepted generation role: %v", err)
	}
	edge.Role = ""
	op := uuid.New()
	doc.Nodes[0].LastOperationID = &op
	if _, err := domain.Apply(doc, []domain.Command{{Type: "Connect", Edges: []domain.Edge{edge}}}); !errors.Is(err, domain.ErrUnsupportedCommand) {
		t.Fatalf("layout connection accepted protected source: %v", err)
	}
}
