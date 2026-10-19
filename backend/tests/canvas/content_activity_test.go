package canvas_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestContentActivityIgnoresViewportAndTransportVersions(t *testing.T) {
	id := uuid.New()
	text := "保留文字"
	before := domain.Document{Revision: 1, Nodes: []domain.Node{{ID: id, Title: "节点", NodeType: "text", NodeAction: "resource", Config: domain.NodeConfig{Text: &text}}}}
	after := before
	after.Revision = 2
	after.Name = "新的项目名称"
	after.Viewport = domain.Viewport{X: 50, Y: 30, Zoom: 2}
	after.Nodes = append([]domain.Node(nil), before.Nodes...)
	after.Nodes[0].InputRevision = 2
	op := uuid.New()
	after.Nodes[0].LastOperationID = &op
	if domain.HasContentChanges(before, after) {
		t.Fatal("transport/view change advanced content activity")
	}
	after.Nodes[0].X = 15
	if !domain.HasContentChanges(before, after) {
		t.Fatal("real layout change did not advance activity")
	}
	if before.Nodes[0].X != 0 || before.Nodes[0].InputRevision != 0 {
		t.Fatal("content comparison mutated its input")
	}
}
