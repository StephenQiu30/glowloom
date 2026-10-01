package canvas_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func generationNode() domain.Node {
	n := batchNode(nil)
	n.NodeType = "generation"
	n.Config = domain.NodeConfig{Generation: &domain.GenerationConfig{Version: 1, Capability: "image.generate", OutputCount: 1, Params: json.RawMessage(`{}`), Inputs: []domain.GenerationReference{}}}
	return n
}

func TestGenerationDraftPersistsInputsWithoutExecutionState(t *testing.T) {
	n := generationNode()
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}})
	if err != nil || doc.Nodes[0].Config.Generation.OutputCount != 1 {
		t.Fatalf("draft rejected: %v", err)
	}
	for _, change := range []func(*domain.GenerationConfig){
		func(c *domain.GenerationConfig) { c.OutputCount = 0 },
		func(c *domain.GenerationConfig) { c.Params = json.RawMessage(`{"task_id":"fake"}`) },
		func(c *domain.GenerationConfig) { c.Capability = "" },
	} {
		n = generationNode()
		change(n.Config.Generation)
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}}); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("invalid draft accepted: %v", err)
		}
	}
}
