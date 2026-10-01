package canvas_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func batchNode(imageID *uuid.UUID) domain.Node {
	return domain.Node{ID: uuid.New(), Title: "批量创作", NodeType: "batch_table", NodeAction: "tool", Config: domain.NodeConfig{BatchTable: &domain.BatchTableConfig{
		Version: 1, Operation: "creative", Concurrency: 3, OutputCount: 1, Params: json.RawMessage(`{}`),
		ReferenceColumns: []domain.BatchReferenceColumn{{ID: uuid.New(), Label: "参考图 1"}},
		Rows:             []domain.BatchTableRow{{ID: uuid.New(), Enabled: true, Prompt: "服装", InputNodeIDs: []*uuid.UUID{imageID}}},
	}}}
}

func TestBatchTablePersistsNullableSlotsAndDeletesReferencesWithoutMutatingInput(t *testing.T) {
	image := resourceNode("image")
	batch := batchNode(&image.ID)
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{batch, image}}})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := domain.Apply(doc, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{image.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Nodes) != 1 || len(deleted.Nodes[0].Config.BatchTable.Rows[0].InputNodeIDs) != 1 || deleted.Nodes[0].Config.BatchTable.Rows[0].InputNodeIDs[0] != nil {
		t.Fatal("delete did not preserve an empty reference slot")
	}
	if doc.Nodes[0].Config.BatchTable.Rows[0].InputNodeIDs[0] == nil {
		t.Fatal("delete mutated authoritative input")
	}
}

func TestBatchTableRejectsForeignReferencesAndExecutionFacts(t *testing.T) {
	image, text := resourceNode("image"), resourceNode("text")
	for _, tc := range []struct {
		name   string
		change func(*domain.Node)
	}{
		{"foreign reference", func(n *domain.Node) { n.Config.BatchTable.Rows[0].InputNodeIDs[0] = ptr(uuid.New()) }},
		{"text reference", func(n *domain.Node) { n.Config.BatchTable.Rows[0].InputNodeIDs[0] = &text.ID }},
		{"resource action", func(n *domain.Node) { n.NodeAction = "resource" }},
		{"mixed config", func(n *domain.Node) { n.Config.Text = ptr("hidden") }},
		{"execution facts", func(n *domain.Node) { n.Config.BatchTable.Params = json.RawMessage(`{"status":"succeeded"}`) }},
		{"null params", func(n *domain.Node) { n.Config.BatchTable.Params = json.RawMessage(`null`) }},
		{"duplicate rows", func(n *domain.Node) {
			n.Config.BatchTable.Rows = append(n.Config.BatchTable.Rows, n.Config.BatchTable.Rows[0])
		}},
		{"missing slot", func(n *domain.Node) { n.Config.BatchTable.Rows[0].InputNodeIDs = nil }},
		{"nil model identity", func(n *domain.Node) { n.Config.BatchTable.ModelProfileID = ptr(uuid.Nil) }},
		{"zero outputs", func(n *domain.Node) { n.Config.BatchTable.OutputCount = 0 }},
		{"too many outputs", func(n *domain.Node) { n.Config.BatchTable.OutputCount = 9 }},
		{"mode too long", func(n *domain.Node) { n.Config.BatchTable.Mode = strings.Repeat("a", 129) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			batch := batchNode(&image.ID)
			tc.change(&batch)
			_, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image, text, batch}}})
			if !errors.Is(err, domain.ErrInvalidCommand) && !errors.Is(err, domain.ErrUnsupportedCommand) {
				t.Fatalf("invalid table accepted: %v", err)
			}
		})
	}
}

func TestBatchTableBoundsCombinedPayloadAndAllowsFiveHundredRows(t *testing.T) {
	batch := batchNode(nil)
	batch.Config.BatchTable.Rows = nil
	for range 500 {
		batch.Config.BatchTable.Rows = append(batch.Config.BatchTable.Rows, domain.BatchTableRow{ID: uuid.New(), Enabled: true, InputNodeIDs: []*uuid.UUID{nil}})
	}
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{batch}}}); err != nil {
		t.Fatal(err)
	}
	for i := range batch.Config.BatchTable.Rows {
		batch.Config.BatchTable.Rows[i].Prompt = strings.Repeat("x", 10000)
	}
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{batch}}}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("oversized combined table accepted: %v", err)
	}
}
