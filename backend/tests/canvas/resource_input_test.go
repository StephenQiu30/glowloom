package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"

	"github.com/google/uuid"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestResourceInputSeparatesPromptFromResult(t *testing.T) {
	for _, kind := range []string{"text", "image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			doc := otDoc()
			n := resourceNode(kind)
			n.Config.Editor = &domain.EditorConfig{SchemaVersion: 1}
			n.RefType, n.RefID = "", nil
			doc.Nodes = []domain.Node{n}
			draft := composerNode(t).Config.Draft
			body, _ := json.Marshal(map[string]any{"draft": draft})
			out, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: raw(`{}`), Value: body}})
			if err != nil {
				t.Fatal(err)
			}
			got := out.Document.Nodes[0]
			if got.InputRevision != 1 || len(out.Inputs) != 1 || got.NodeType != kind || got.NodeAction != "resource" {
				t.Fatalf("invalid input owner: %+v", got)
			}
			if kind == "text" && (got.Config.Text == nil || *got.Config.Text != *n.Config.Text) {
				t.Fatal("prompt overwrote output")
			}
			moved, err := domain.ApplyOperations(out.Document, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"position"}, Value: raw(`{"x":10,"y":20}`)}})
			if err != nil || moved.Document.Nodes[0].InputRevision != 1 || len(moved.Inputs) != 0 {
				t.Fatalf("layout changed input: %v", err)
			}
			next := composerNode(t).Config.Draft
			next.Composer.Params["temperature"] = json.RawMessage(`0.4`)
			nextBody, _ := json.Marshal(map[string]any{"draft": next})
			edited, err := domain.ApplyOperations(moved.Document, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: body, Value: nextBody}})
			if err != nil || edited.Document.Nodes[0].InputRevision != 2 || len(edited.Inputs) != 1 {
				t.Fatalf("input change not versioned: %v", err)
			}
			if _, err := domain.ApplyOperations(edited.Document, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: body, Value: nextBody}}); err == nil {
				t.Fatal("stale input replaced current draft")
			}
		})
	}
}

func TestResourceInputDeletionKeepsTimelineAsset(t *testing.T) {
	image := resourceNode("image")
	image.Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Files: []domain.EditorFileReference{{FileID: *image.RefID, Purpose: "CONTENT", ReferenceKey: "result", Version: "1"}}}
	image.Config.Draft = composerNode(t).Config.Draft
	timeline := timelineNode()
	timeline.Config.Timeline.Tracks[0].Kind = "image"
	timeline.Config.Timeline.Clips[0].Kind = "image"
	timeline.Config.Timeline.Clips[0].NodeID = &image.ID
	created, err := domain.ApplyOperations(otDoc(), []domain.Operation{{Type: "create_node", Node: &image}, {Type: "create_node", Node: &timeline}})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := domain.ApplyOperations(created.Document, []domain.Operation{{Type: "delete_node", NodeID: image.ID}})
	if err != nil {
		t.Fatalf("resource input changed deletion semantics: %v", err)
	}
	for _, node := range deleted.Document.Nodes {
		if node.ID != timeline.ID {
			continue
		}
		clip := node.Config.Timeline.Clips[0]
		if clip.NodeID != nil || clip.AssetID == nil || *clip.AssetID != *image.RefID {
			t.Fatal("deleting input owner lost timeline media identity")
		}
	}
}

func resourceInputNode(t *testing.T, kind string) domain.Node {
	t.Helper()
	n := resourceNode(kind)
	n.RefType, n.RefID = "", nil
	n.Config.Editor = &domain.EditorConfig{SchemaVersion: 1}
	n.Config.Draft = composerNode(t).Config.Draft
	return n
}

func TestResourceInputCreateRestoreAndCopy(t *testing.T) {
	for _, kind := range []string{"text", "image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			n := resourceInputNode(t, kind)
			created, err := domain.ApplyOperations(otDoc(), []domain.Operation{{Type: "create_node", Node: &n}})
			if err != nil || len(created.Inputs) != 1 || created.Inputs[0].Revision != 1 {
				t.Fatal("first input snapshot", created.Inputs, err)
			}
			n = created.Document.Nodes[1]
			deleted, err := domain.ApplyOperations(created.Document, []domain.Operation{{Type: "delete_node", NodeID: n.ID}})
			if err != nil || len(deleted.Inputs) != 0 {
				t.Fatal("delete created input", err)
			}
			restored, err := domain.ApplyOperations(deleted.Document, []domain.Operation{{Type: "restore_node", Node: &n}})
			if err != nil || len(restored.Inputs) != 1 || restored.Inputs[0].Revision != 2 || restored.Document.Nodes[1].ID != n.ID {
				t.Fatal("restore reused input revision or changed owner", restored.Inputs, err)
			}
			// A duplicate is a fresh owner; it must not inherit source input versions.
			duplicate := n
			duplicate.ID, duplicate.InputRevision = uuid.New(), 0
			duplicated, err := domain.ApplyOperations(restored.Document, []domain.Operation{{Type: "create_node", Node: &duplicate}})
			if err != nil || len(duplicated.Inputs) != 1 || duplicated.Inputs[0].NodeID != duplicate.ID || duplicated.Inputs[0].Revision != 1 {
				t.Fatal("copy did not establish independent input", duplicated.Inputs, err)
			}
		})
	}
}

func TestResourceInputRejectsUnsupportedOwnersAndLegacyFields(t *testing.T) {
	for _, kind := range []string{"model", "group", "future"} {
		t.Run(kind, func(t *testing.T) {
			n := resourceInputNode(t, kind)
			if _, err := domain.ApplyOperations(otDoc(), []domain.Operation{{Type: "create_node", Node: &n}}); err == nil {
				t.Fatal("unsupported input owner accepted")
			}
		})
	}
	for _, kind := range []string{"text", "image", "video", "audio"} {
		n := resourceInputNode(t, kind)
		n.Config.Draft = &domain.Draft{Prompt: "legacy", AspectRatio: "1:1", CandidateCount: 1}
		if _, err := domain.ApplyOperations(otDoc(), []domain.Operation{{Type: "create_node", Node: &n}}); err == nil {
			t.Fatal("resource accepted legacy generation input", kind)
		}
		n = resourceInputNode(t, kind)
		n.LastOperationID = ptr(uuid.New())
		doc := otDoc()
		doc.Nodes = []domain.Node{n}
		if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "delete_node", NodeID: n.ID}}); err == nil {
			t.Fatal("protected execution result deleted", kind)
		}
	}
}

func TestResourceInputLabelsAndPresentationDoNotAdvanceInput(t *testing.T) {
	n := resourceInputNode(t, "text")
	created, err := domain.ApplyOperations(otDoc(), []domain.Operation{{Type: "create_node", Node: &n}})
	if err != nil {
		t.Fatal(err)
	}
	n = created.Document.Nodes[1]
	before, _ := json.Marshal(map[string]any{"draft": n.Config.Draft})
	next := composerNode(t).Config.Draft
	next.Composer.LabelState.HighWater["image"] = 1
	next.Composer.LabelState.Allocations["removed-reference"] = promptdomain.LabelAllocation{LabelKind: "image", Ordinal: 1}
	after, _ := json.Marshal(map[string]any{"draft": next})
	withLabels, err := domain.ApplyOperations(created.Document, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: before, Value: after}})
	if err != nil || len(withLabels.Inputs) != 0 || withLabels.Document.Nodes[1].InputRevision != 1 {
		t.Fatal("inactive label tombstone advanced input", withLabels.Inputs, err)
	}
	if _, err := domain.ApplyOperations(withLabels.Document, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: after, Value: before}}); err == nil {
		t.Fatal("input label ordinal reclaimed")
	}
	editor, _ := json.Marshal(n.Config.Editor)
	newEditor := &domain.EditorConfig{SchemaVersion: 1, Metadata: map[string]json.RawMessage{"textBackground": raw(`"cyan"`)}}
	updatedEditor, _ := json.Marshal(newEditor)
	presentation, err := domain.ApplyOperations(withLabels.Document, []domain.Operation{
		{Type: "set", NodeID: n.ID, Path: []string{"title"}, Value: raw(`"已改名"`)},
		{Type: "set", NodeID: n.ID, Path: []string{"config", "editor"}, ExpectedExists: ptr(true), Expected: editor, Value: updatedEditor},
		{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "新结果"}}}}},
	})
	if err != nil || len(presentation.Inputs) != 0 || presentation.Document.Nodes[1].InputRevision != 1 {
		t.Fatal("result or presentation changed generation input", presentation.Inputs, err)
	}
}
