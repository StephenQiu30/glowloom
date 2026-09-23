package domain

import (
	"errors"
	"testing"
)

const (
	firstNodeID  = "019fb2d0-a000-7000-8000-000000000101"
	secondNodeID = "019fb2d0-a000-7000-8000-000000000102"
	edgeID       = "019fb2d0-a000-7000-8000-000000000103"
)

func TestApplyOperationsPreservesUnrelatedNodeUpdates(t *testing.T) {
	document := Document{SchemaVersion: 1, Revision: 1, Nodes: []Node{
		{ID: firstNodeID, Kind: "text", Title: "A", X: 0, Y: 0, Width: 280, Height: 180, Revision: 1},
		{ID: secondNodeID, Kind: "image", Title: "B", X: 400, Y: 0, Width: 280, Height: 220, Revision: 1},
	}}
	first := document.Nodes[0]
	first.X = 80
	updated, err := ApplyOperations(document, []Operation{{Kind: "update_node", Node: first, ExpectedRevision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	second := document.Nodes[1]
	second.Y = 60
	merged, err := ApplyOperations(updated, []Operation{{Kind: "update_node", Node: second, ExpectedRevision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Revision != 3 || merged.Nodes[0].X != 80 || merged.Nodes[1].Y != 60 || merged.Nodes[0].Revision != 2 || merged.Nodes[1].Revision != 2 {
		t.Fatalf("unexpected merged document: %+v", merged)
	}
	if document.Nodes[0].X != 0 {
		t.Fatal("original document mutated")
	}
}

func TestApplyOperationsRejectsStaleNodeAndDeletedIdentity(t *testing.T) {
	document := Document{SchemaVersion: 1, Revision: 1, Nodes: []Node{{ID: firstNodeID, Kind: "text", Title: "A", Width: 280, Height: 180, Revision: 2}}}
	stale := document.Nodes[0]
	stale.Title = "stale"
	if _, err := ApplyOperations(document, []Operation{{Kind: "update_node", Node: stale, ExpectedRevision: 1}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
	deleted, err := ApplyOperations(document, []Operation{{Kind: "delete_node", NodeID: firstNodeID, ExpectedRevision: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyOperations(deleted, []Operation{{Kind: "create_node", Node: stale}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected tombstone conflict, got %v", err)
	}
}

func TestApplyOperationsValidatesEdgesAndBatchAtomicity(t *testing.T) {
	initial := Document{SchemaVersion: 1}
	first := Node{ID: firstNodeID, Kind: "image", Title: "图像", Width: 280, Height: 220}
	second := Node{ID: secondNodeID, Kind: "video", Title: "视频", X: 400, Width: 320, Height: 220}
	edge := Connection{ID: edgeID, FromNodeID: firstNodeID, ToNodeID: secondNodeID, Purpose: "visual"}
	document, err := ApplyOperations(initial, []Operation{
		{Kind: "create_node", Node: first},
		{Kind: "create_node", Node: second},
		{Kind: "create_edge", Connection: edge},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 2 || len(document.Connections) != 1 || document.Revision != 1 {
		t.Fatalf("unexpected document: %+v", document)
	}
	invalid := Connection{ID: "019fb2d0-a000-7000-8000-000000000104", FromNodeID: firstNodeID, ToNodeID: "019fb2d0-a000-7000-8000-000000000999", Purpose: "visual"}
	if _, err := ApplyOperations(document, []Operation{{Kind: "create_edge", Connection: invalid}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected invalid missing endpoint, got %v", err)
	}
	if len(document.Connections) != 1 {
		t.Fatal("failed batch changed original document")
	}
}
