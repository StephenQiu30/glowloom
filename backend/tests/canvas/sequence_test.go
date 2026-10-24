package canvas_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestSequenceStrictAtomsAndSemanticPromptRoundTrip(t *testing.T) {
	for _, raw := range []string{`[{"insert":[{"type":"text","text":"a","marks":["bold"]}]}]`, `[{"insert":[{"type":"text","text":"\ud800"}]}]`, `[{"insert":[{"type":"text","text":"a"}],"other":1}]`} {
		var op domain.SequenceEdit
		if err := json.Unmarshal([]byte(raw), &op); err == nil {
			t.Fatalf("lossy unknown/Unicode input accepted: %s", raw)
		}
	}
	var document promptdomain.Document
	if err := json.Unmarshal([]byte(`{"type":"doc","content":[{"type":"paragraph"},{"type":"paragraph","content":[{"type":"text","text":"中😀"},{"type":"hardBreak"},{"type":"reference","attrs":{"referenceId":"r"}},{"type":"reference","attrs":{"referenceId":"r"}}]},{"type":"paragraph"}]}`), &document); err != nil {
		t.Fatal(err)
	}
	seq, err := domain.PromptSequence(document)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := domain.SequencePrompt(seq)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document, rebuilt) {
		t.Fatalf("paragraphs/atoms lost: %+v", rebuilt)
	}
}
func TestSequenceRejectsOriginalInvalidUTF16Boundaries(t *testing.T) {
	before := []domain.Atom{{Type: "text", Text: "😀"}}
	for _, op := range []domain.SequenceEdit{
		{{Retain: 1}, {Delete: 1}}, {{Retain: 3}}, {{Delete: 1}}, {{Retain: -1}}, {{Retain: 1, Delete: 1}}, {{Insert: []domain.Atom{{Type: "unknown"}}}},
	} {
		if _, err := domain.ApplySequence(before, op); err == nil {
			t.Fatalf("invalid original operation accepted: %+v", op)
		}
	}
}
