package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestEditorParameterPersistence(t *testing.T) {
	doc := otDoc()
	config := raw(`{"schemaVersion":1,"parameters":{"mode":"MANUAL","modelParameters":{"temperature":0.7}},"metadata":{"fontSize":28,"freeResize":true}}`)
	op := domain.Operation{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"config", "editor"}, ExpectedExists: ptr(false), Value: config}
	result, err := domain.ApplyOperations(doc, []domain.Operation{op})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(result.Document)
	var reopened domain.Document
	if err = json.Unmarshal(body, &reopened); err != nil {
		t.Fatal(err)
	}
	if err = domain.ValidateGraph(reopened); err != nil {
		t.Fatal(err)
	}
	if _, err = domain.ApplyOperations(reopened, []domain.Operation{op}); err == nil {
		t.Fatal("stale parameters overwrote saved state")
	}
}

func TestEditorTextBackgroundPersistence(t *testing.T) {
	for _, color := range []string{"default", "cyan", "blue", "purple", "orange", "yellow"} {
		t.Run(color, func(t *testing.T) {
			doc := otDoc()
			doc.Nodes[0].Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Metadata: map[string]json.RawMessage{"textBackground": raw(`"` + color + `"`)}}
			if err := domain.ValidateGraph(doc); err != nil {
				t.Fatalf("valid text background rejected: %v", err)
			}
			body, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var reopened domain.Document
			if err := json.Unmarshal(body, &reopened); err != nil {
				t.Fatal(err)
			}
			if string(reopened.Nodes[0].Config.Editor.Metadata["textBackground"]) != `"`+color+`"` {
				t.Fatal("text background lost on reopen")
			}
		})
	}
	for _, invalid := range []string{`"#ffffff"`, `"url(https://example.com)"`, `""`, `1`, `null`, `{}`} {
		doc := otDoc()
		doc.Nodes[0].Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Metadata: map[string]json.RawMessage{"textBackground": raw(invalid)}}
		if domain.ValidateGraph(doc) == nil {
			t.Fatalf("accepted invalid background %s", invalid)
		}
	}
}
func TestEditorEmptyMediaAndBindingPersistence(t *testing.T) {
	doc := otDoc()
	id := uuid.New()
	edge := uuid.New()
	body := raw(`{"id":"` + id.String() + `","node_type":"image","node_action":"resource","title":"图片","x":0,"y":0,"width":320,"height":240,"config":{"editor":{"schemaVersion":1,"parameters":{"mode":"GENERATE"}}}}`)
	var node domain.Node
	if err := json.Unmarshal(body, &node); err != nil {
		t.Fatal(err)
	}
	binding := raw(`{"schemaVersion":1,"fromPortId":"text","toPortId":"prompt","purpose":"PROMPT","order":0,"binding":{"kind":"NODE_VALUE"}}`)
	result, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "create_node", Node: &node}, {Type: "connect", Edge: &domain.Edge{ID: edge, EdgeType: "annotation", SourceNodeID: doc.Nodes[0].ID, TargetNodeID: id, Binding: binding}}})
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := json.Marshal(result.Document)
	var reopened domain.Document
	if err = json.Unmarshal(bytes, &reopened); err != nil {
		t.Fatal(err)
	}
	if err = domain.ValidateGraph(reopened); err != nil {
		t.Fatal(err)
	}
	if string(reopened.Edges[0].Binding) != string(binding) {
		t.Fatal("binding lost")
	}
}

func TestEditorConnectionsRejectInvalidGraphAndStaleEdit(t *testing.T) {
	doc := otDoc()
	second := resourceNode("text")
	doc.Nodes = append(doc.Nodes, second)
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: doc.Nodes[0].ID, TargetNodeID: second.ID, Binding: raw(`{"schemaVersion":1,"fromPortId":"text","toPortId":"prompt","purpose":"PROMPT","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	saved, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "connect", Edge: &edge}})
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range []string{
		`{"schemaVersion":1,"fromPortId":"image","toPortId":"reference_images","purpose":"REFERENCE_IMAGE","order":0,"binding":{"kind":"FILE_REFERENCE","referenceKey":"missing"}}`,
		`{"schemaVersion":1,"fromPortId":"text","toPortId":"settings","purpose":"SETTINGS","order":0,"binding":{"kind":"NODE_VALUE"}}`,
	} {
		bad := edge
		bad.ID = uuid.New()
		bad.Binding = raw(binding)
		if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "connect", Edge: &bad}}); err == nil {
			t.Fatal("accepted invalid binding")
		}
	}
	cycle := edge
	cycle.ID = uuid.New()
	cycle.SourceNodeID, cycle.TargetNodeID = edge.TargetNodeID, edge.SourceNodeID
	if _, err := domain.ApplyOperations(saved.Document, []domain.Operation{{Type: "connect", Edge: &cycle}}); err == nil {
		t.Fatal("accepted cycle")
	}
	changed := edge
	changed.Binding = raw(`{"schemaVersion":1,"fromPortId":"text","toPortId":"prompt","purpose":"PROMPT","order":2,"binding":{"kind":"NODE_VALUE"}}`)
	expected, _ := json.Marshal(edge)
	op := domain.Operation{Type: "set_edge", NodeID: edge.ID, Edge: &changed, Expected: expected}
	updated, err := domain.ApplyOperations(saved.Document, []domain.Operation{op})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ApplyOperations(updated.Document, []domain.Operation{op}); err == nil {
		t.Fatal("accepted stale edge")
	}
	restored, err := domain.RewindEffects(updated.Document, updated.Effects)
	if err != nil || string(restored.Edges[0].Binding) != string(edge.Binding) {
		t.Fatal("cannot restore edge", err)
	}
}

func TestEditorEmptyMediaBindAndClear(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio", "image_generation"} {
		t.Run(kind, func(t *testing.T) {
			doc := otDoc()
			n := resourceNode(kind)
			if kind == "image_generation" {
				n = draftNode(t, kind)
			}
			n.RefID = nil
			n.RefType = ""
			n.Config.Editor = &domain.EditorConfig{SchemaVersion: 1}
			doc.Nodes = append(doc.Nodes, n)
			asset := uuid.New()
			editor := domain.EditorConfig{SchemaVersion: 1, Files: []domain.EditorFileReference{{ReferenceKey: "stable", FileID: asset, Version: "1", Purpose: "CONTENT"}}}
			before, _ := json.Marshal(n.Config.Editor)
			value, _ := json.Marshal(editor)
			out, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "editor"}, ExpectedExists: ptr(true), Expected: before, Value: value}, {Type: "replace_media", NodeID: n.ID, ExpectedMediaRef: &domain.MediaRef{Empty: true}, NewMediaRef: &domain.MediaRef{AssetID: asset}}})
			if err != nil {
				t.Fatal(err)
			}
			cleared, err := domain.ApplyOperations(out.Document, []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"config", "editor"}, ExpectedExists: ptr(true), Expected: value, Value: before}, {Type: "replace_media", NodeID: n.ID, ExpectedMediaRef: &domain.MediaRef{AssetID: asset}, NewMediaRef: &domain.MediaRef{Empty: true}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range cleared.Document.Nodes {
				if n.ID == doc.Nodes[1].ID && n.RefID != nil {
					t.Fatal("clear retained content")
				}
			}
		})
	}
}

func TestEditorVideoLastFrameBinding(t *testing.T) {
	video, target := resourceNode("video"), resourceNode("video")
	frame := uuid.New()
	video.Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Files: []domain.EditorFileReference{{ReferenceKey: "video", FileID: *video.RefID, Version: "1", Purpose: "CONTENT"}, {ReferenceKey: "frame", FileID: frame, Version: "1", Purpose: "LAST_FRAME"}}}
	target.RefID = nil
	target.RefType = ""
	target.Config.Editor = &domain.EditorConfig{SchemaVersion: 1}
	doc := domain.Document{ID: uuid.New(), Revision: 1, Nodes: []domain.Node{video, target}}
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: video.ID, TargetNodeID: target.ID, Binding: raw(`{"schemaVersion":1,"fromPortId":"last_frame","toPortId":"first_frame","purpose":"FIRST_FRAME","order":0,"binding":{"kind":"FILE_REFERENCE","referenceKey":"frame"}}`)}
	if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "connect", Edge: &edge}}); err != nil {
		t.Fatal(err)
	}
	edge.Binding = raw(`{"schemaVersion":1,"fromPortId":"last_frame","toPortId":"first_frame","purpose":"FIRST_FRAME","order":0,"binding":{"kind":"FILE_REFERENCE","referenceKey":"video"}}`)
	if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "connect", Edge: &edge}}); err == nil {
		t.Fatal("video file used as extracted frame")
	}
}
