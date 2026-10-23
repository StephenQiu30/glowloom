package canvas_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

const textComposerJSON = `{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"参考😀"},{"type":"reference","attrs":{"referenceId":"image-one"}},{"type":"reference","attrs":{"referenceId":"image-one"}}]}]},"references":[{"id":"image-one","source":{"kind":"pending_upload","clientUploadId":"upload-one","uploadAttemptId":"attempt-one"},"bindingGeneration":1,"mediaKind":"image","role":"reference_image","order":0}],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":1,"video":0,"audio":0,"subject":0},"allocations":{"image-one":{"labelKind":"image","ordinal":1}}}}`

func TestCanvasFormattedTextRoundTrip(t *testing.T) {
	doc := otDoc()
	doc.Nodes[0].Config.Text = ptr("before")
	rich := `{"schemaVersion":1,"content":{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"参考😀","marks":[{"type":"bold"}]},{"type":"reference","attrs":{"referenceId":"image-one"}},{"type":"reference","attrs":{"referenceId":"image-one"}}]}]}}`
	value := raw(`{"format":"tiptap","format_version":1,"composer":` + textComposerJSON + `,"rich_text":` + rich + `}`)
	op := domain.Operation{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text_content"}, ExpectedExists: ptr(true), Expected: raw(`{"text":"before","format":"plain","format_version":1}`), Value: value}
	saved, err := domain.ApplyOperations(doc, []domain.Operation{op})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(saved.Document)
	var reopened domain.Document
	if err := json.Unmarshal(body, &reopened); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateGraph(reopened); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(reopened.Nodes[0].Config)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(config, &fields)
	var got, want any
	_ = json.Unmarshal(fields["rich_text"], &got)
	_ = json.Unmarshal([]byte(rich), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatted reference content changed: %s", config)
	}
}

func TestTextComposerSaveRoundTripAndConflict(t *testing.T) {
	doc := otDoc()
	doc.Nodes[0].Config.Text = ptr("before")
	op := domain.Operation{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text_content"}, ExpectedExists: ptr(true), Expected: raw(`{"text":"before","format":"plain","format_version":1}`), Value: raw(`{"format":"tiptap","format_version":1,"composer":` + textComposerJSON + `}`)}
	saved, err := domain.ApplyOperations(doc, []domain.Operation{op})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Inputs) != 0 {
		t.Fatal("text resource created a generation snapshot")
	}
	body, err := json.Marshal(saved.Document)
	if err != nil {
		t.Fatal(err)
	}
	var reopened domain.Document
	if err := json.Unmarshal(body, &reopened); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateGraph(reopened); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(reopened.Nodes[0].Config)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(config, &fields)
	var got, want any
	_ = json.Unmarshal(fields["composer"], &got)
	_ = json.Unmarshal([]byte(textComposerJSON), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reference identity or labels lost: %s", fields["composer"])
	}
	if _, err := domain.ApplyOperations(reopened, []domain.Operation{op}); err == nil {
		t.Fatal("stale save overwrote references")
	}
	if _, err := domain.ApplyOperations(reopened, []domain.Operation{{Type: "text", NodeID: op.NodeID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "flatten"}}}}}}); err == nil {
		t.Fatal("legacy plain editor overwrote rich content")
	}
}

func TestTextComposerUndoKeepsLabelAllocations(t *testing.T) {
	doc := otDoc()
	config := raw(`{"format":"tiptap","format_version":1,"composer":` + textComposerJSON + `}`)
	if err := json.Unmarshal(config, &doc.Nodes[0].Config); err != nil {
		t.Fatal(err)
	}
	op := domain.Operation{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text_content"}, ExpectedExists: ptr(true), Expected: config, Value: raw(`{"format":"plain","format_version":1,"text":"before"}`)}
	result, err := domain.ApplyOperations(doc, []domain.Operation{op})
	if err != nil {
		t.Fatal(err)
	}
	rich := result.Document.Nodes[0].Config.Composer
	if rich == nil || rich.LabelState.Allocations["image-one"].Ordinal != 1 || len(rich.References) != 0 {
		t.Fatal("undo discarded stable number allocation", rich)
	}
}

func TestTextComposerCannotReassignLabelsOrCopyUnmappedReferences(t *testing.T) {
	doc := otDoc()
	doc.Name = "Text copy"
	doc.Scope = raw(`{}`)
	doc.Viewport.Zoom = 1
	config := raw(`{"format":"tiptap","format_version":1,"composer":` + textComposerJSON + `}`)
	if err := json.Unmarshal(config, &doc.Nodes[0].Config); err != nil {
		t.Fatal(err)
	}
	next := doc.Nodes[0].Config
	if err := json.Unmarshal(config, &next); err != nil {
		t.Fatal(err)
	}
	allocation := next.Composer.LabelState.Allocations["image-one"]
	allocation.Ordinal = 2
	next.Composer.LabelState.Allocations["image-one"] = allocation
	next.Composer.LabelState.HighWater["image"] = 2
	body, _ := json.Marshal(next)
	if _, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: doc.Nodes[0].ID, Path: []string{"config", "text_content"}, ExpectedExists: ptr(true), Expected: config, Value: body}}); err == nil {
		t.Fatal("reference label was reassigned")
	}
	if _, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil); !errors.Is(err, domain.ErrUnsupportedProjectCopy) {
		t.Fatal("unmapped reference copied across projects", err)
	}
	doc.Nodes[0].Config.Composer.References = []prompt.Reference{}
	doc.Nodes[0].Config.Composer.Document.Content[0].Content = doc.Nodes[0].Config.Composer.Document.Content[0].Content[:1]
	copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil)
	if err != nil || !reflect.DeepEqual(copied.Document.Nodes[0].Config.Composer, doc.Nodes[0].Config.Composer) {
		t.Fatal("reference-free Tiptap text copy lost content", err)
	}
}
