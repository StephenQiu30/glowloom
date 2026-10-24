package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestEditorCompleteGraphDurableRoundTrip(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	store := canvasStore(db)
	write, read := canvasfixture.NewEditService(store), application.NewService(store)
	doc, err := read.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "完整保存验证"})
	if err != nil {
		t.Fatal(err)
	}
	submit := func(base int64, ops []domain.Operation, undo string) canvasfixture.EditReceipt {
		t.Helper()
		r, e := write.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, Operations: ops, UndoOf: undo})
		if e != nil || r.Status != "accepted" {
			t.Fatalf("save: %+v %v", r, e)
		}
		return r
	}
	image, text, config := resourceNode("image"), resourceNode("text"), resourceNode("config")
	old, next := seedCanvasMedia(t, db, project, "image"), seedCanvasMedia(t, db, project, "image")
	image.RefID = &old
	image.Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Parameters: &domain.EditorParameters{Mode: "MANUAL"}, Files: []domain.EditorFileReference{{ReferenceKey: "stable-reference", FileID: old, Version: "1", Purpose: "CONTENT"}}}
	text.Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Parameters: &domain.EditorParameters{Mode: "GENERATE", ModelSelection: &domain.EditorModelSelection{ConnectionID: "10000000-0000-4000-8000-000000000011", ModelID: "model"}, ModelParameters: map[string]json.RawMessage{"temperature": raw(`0.7`)}}, Metadata: map[string]json.RawMessage{"fontSize": raw(`28`)}}
	config.Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Parameters: &domain.EditorParameters{Values: map[string]json.RawMessage{"quality": raw(`"high"`)}}}
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: image.ID, TargetNodeID: text.ID, Binding: raw(`{"schemaVersion":1,"fromPortId":"image","toPortId":"reference_images","purpose":"REFERENCE_IMAGE","order":0,"binding":{"kind":"FILE_REFERENCE","referenceKey":"stable-reference"}}`)}
	settings := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: config.ID, TargetNodeID: text.ID, Binding: raw(`{"schemaVersion":1,"fromPortId":"settings","toPortId":"settings","purpose":"SETTINGS","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	submit(1, []domain.Operation{{Type: "create_node", Node: &image}, {Type: "create_node", Node: &text}, {Type: "create_node", Node: &config}, {Type: "connect", Edge: &edge}, {Type: "connect", Edge: &settings}}, "")
	reopened, err := application.NewService(canvasStore(db)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(reopened.Nodes) != 3 || len(reopened.Edges) != 2 {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
	for _, e := range reopened.Edges {
		if len(e.Binding) == 0 {
			t.Fatal("lost port binding")
		}
	}
	_, foreignProject := seedCanvasActor(t, db)
	foreign := seedCanvasMedia(t, db, foreignProject, "image")
	guarded := *image.Config.Editor
	guarded.Files = append(append([]domain.EditorFileReference{}, guarded.Files...), domain.EditorFileReference{ReferenceKey: "foreign", FileID: foreign, Version: "1", Purpose: "CONTENT"})
	guardedValue, _ := json.Marshal(guarded)
	guardedExpected, _ := json.Marshal(image.Config.Editor)
	rejected, rejectErr := write.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "set", NodeID: image.ID, Path: []string{"config", "editor"}, ExpectedExists: ptr(true), Expected: guardedExpected, Value: guardedValue}}})
	if rejectErr != nil || rejected.Status != "rejected" {
		t.Fatalf("foreign secondary file accepted: %+v %v", rejected, rejectErr)
	}
	unchanged, readErr := read.Get(t.Context(), actor, doc.ID)
	if readErr != nil || unchanged.Revision != 2 {
		t.Fatal("rejected save changed graph", readErr)
	}
	before, _ := json.Marshal(image.Config.Editor)
	image.Config.Editor.Files[0].FileID = next
	value, _ := json.Marshal(image.Config.Editor)
	updated := edge
	updated.Binding = raw(`{"schemaVersion":1,"fromPortId":"image","toPortId":"reference_images","purpose":"REFERENCE_IMAGE","order":3,"binding":{"kind":"FILE_REFERENCE","referenceKey":"stable-reference"}}`)
	expected, _ := json.Marshal(edge)
	receipt := submit(2, []domain.Operation{{Type: "set", NodeID: image.ID, Path: []string{"config", "editor"}, ExpectedExists: ptr(true), Expected: before, Value: value}, mediaReplacement(t, image.ID, old, next), {Type: "set_edge", NodeID: edge.ID, Expected: expected, Edge: &updated}}, "")
	reopened, err = application.NewService(canvasStore(db)).Get(t.Context(), actor, doc.ID)
	if err != nil || reopened.Revision != 3 {
		t.Fatal(err, reopened.Revision)
	}
	for _, n := range reopened.Nodes {
		if n.ID == image.ID && (n.RefID == nil || *n.RefID != next || n.Config.Editor.Files[0].ReferenceKey != "stable-reference" || n.Config.Editor.Files[0].FileID != next) {
			t.Fatal("lost replacement", n)
		}
	}
	for _, e := range reopened.Edges {
		if e.ID == edge.ID {
			var c domain.EditorConnection
			if json.Unmarshal(e.Binding, &c) != nil || c.Order != 3 {
				t.Fatal("lost edited edge", e)
			}
		}
	}
	submit(3, nil, receipt.OpID)
	reopened, err = read.Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range reopened.Nodes {
		if n.ID == image.ID && (n.RefID == nil || *n.RefID != old || n.Config.Editor.Files[0].FileID != old) {
			t.Fatal("undo lost reference", n)
		}
	}
	for _, id := range []uuid.UUID{old, next} {
		if retained, e := canvasReferenceCheck(t, db, actor, project, id); e != nil || !retained {
			t.Fatal("history retention lost", id, e)
		}
	}
}
