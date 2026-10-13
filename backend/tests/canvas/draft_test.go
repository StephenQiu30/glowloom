package canvas_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func draftNode(t *testing.T, kind string) domain.Node {
	t.Helper()
	raw := `{"id":"` + uuid.NewString() + `","title":"Draft","node_type":"` + kind + `","node_action":"generate","config":{"draft":{"prompt":"First","negative_prompt":"","aspect_ratio":"1:1","candidate_count":1}},"x":0,"y":0,"z_index":0}`
	var node domain.Node
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatal(err)
	}
	return node
}

func inputRevision(t *testing.T, node domain.Node) int {
	t.Helper()
	raw, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Revision int `json:"input_revision"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value.Revision
}

func TestM1DraftInputsAdvanceOnlyOnContent(t *testing.T) {
	for _, kind := range []string{"image_generation", "video_generation"} {
		t.Run(kind, func(t *testing.T) {
			node := draftNode(t, kind)
			doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
			if err != nil || len(doc.Nodes) != 1 || inputRevision(t, doc.Nodes[0]) != 1 {
				t.Fatal("draft creation must initialize its input", err)
			}
			moved, err := domain.Apply(doc, []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: node.ID, X: 100, Y: -20}}}})
			if err != nil || inputRevision(t, moved.Nodes[0]) != 1 || doc.Nodes[0].X != 0 {
				t.Fatal("movement changed input or mutated original", err)
			}
			resized, err := domain.Apply(moved, []domain.Command{{Type: "ResizeNodes", Sizes: []domain.Size{{ID: node.ID, Width: 400, Height: 260}}}})
			if err != nil || inputRevision(t, resized.Nodes[0]) != 1 || *resized.Nodes[0].Width != 400 || moved.Nodes[0].Width != nil {
				t.Fatal("resizing changed input or mutated original", err)
			}
			var config domain.NodeConfig
			if err := json.Unmarshal([]byte(`{"draft":{"prompt":"Second","negative_prompt":"","aspect_ratio":"1:1","candidate_count":1}}`), &config); err != nil {
				t.Fatal(err)
			}
			updated, err := domain.Apply(resized, []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &config}})
			if err != nil || inputRevision(t, updated.Nodes[0]) != 2 || inputRevision(t, moved.Nodes[0]) != 1 {
				t.Fatal("draft content revision lost", err)
			}
			again, err := domain.Apply(updated, []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &config}})
			if err != nil || inputRevision(t, again.Nodes[0]) != 2 {
				t.Fatal("equal draft advanced input", err)
			}
			if _, err := domain.Apply(again, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{node.ID}}}); !errors.Is(err, domain.ErrUnsupportedCommand) {
				t.Fatal("draft history deleted", err)
			}
		})
	}
}

func TestM1DraftRejectsClientInputRevisionAndMalformedParams(t *testing.T) {
	for _, patch := range []string{`"input_revision":9`, `"config":{"draft":{"prompt":"x","aspect_ratio":"2:3","candidate_count":1}}`, `"config":{"draft":{"prompt":"x","aspect_ratio":"1:1","candidate_count":0}}`, `"ref_type":"media_asset","ref_id":"` + uuid.NewString() + `"`} {
		node := draftNode(t, "image_generation")
		raw, _ := json.Marshal(node)
		var object map[string]json.RawMessage
		_ = json.Unmarshal(raw, &object)
		var replacement map[string]json.RawMessage
		if err := json.Unmarshal([]byte("{"+patch+"}"), &replacement); err != nil {
			t.Fatal(err)
		}
		for key, value := range replacement {
			object[key] = value
		}
		raw, _ = json.Marshal(object)
		if err := json.Unmarshal(raw, &node); err != nil {
			t.Fatal(err)
		}
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
			t.Fatal("invalid draft accepted", patch)
		}
	}
}
