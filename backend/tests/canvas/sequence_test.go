package canvas_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestSequenceOTGoldenIntentPreservation(t *testing.T) {
	data, err := os.ReadFile("testdata/sequence.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name   string
		Before []domain.Atom
		A, B   domain.SequenceEdit
		Want   []domain.Atom
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	extra, err := os.ReadFile("testdata/sequence-random.json")
	if err != nil {
		t.Fatal(err)
	}
	var randomVectors []struct {
		Name   string
		Before []domain.Atom
		A, B   domain.SequenceEdit
		Want   []domain.Atom
	}
	if err := json.Unmarshal(extra, &randomVectors); err != nil {
		t.Fatal(err)
	}
	vectors = append(vectors, randomVectors...)
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			ap, bp, err := domain.TransformSequencePair(v.A, v.B, domain.SequenceLength(v.Before))
			if err != nil {
				t.Fatal(err)
			}
			afterA, err := domain.ApplySequence(v.Before, v.A)
			if err != nil {
				t.Fatal(err)
			}
			ab, err := domain.ApplySequence(afterA, bp)
			if err != nil {
				t.Fatal(err)
			}
			afterB, err := domain.ApplySequence(v.Before, v.B)
			if err != nil {
				t.Fatal(err)
			}
			ba, err := domain.ApplySequence(afterB, ap)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ab, v.Want) || !reflect.DeepEqual(ba, v.Want) {
				t.Fatalf("compatible intent lost: AB=%+v BA=%+v want=%+v", ab, ba, v.Want)
			}
		})
	}
}
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

func TestSequenceInversePreservesExactDeletedContribution(t *testing.T) {
	before := []domain.Atom{{Type: "text", Text: "A😀BC"}}
	edit := domain.SequenceEdit{{Retain: 1}, {Insert: []domain.Atom{{Type: "text", Text: "中"}}}, {Delete: 3}}
	inverse, err := domain.InvertSequence(before, edit)
	if err != nil {
		t.Fatal(err)
	}
	after, err := domain.ApplySequence(before, edit)
	if err != nil {
		t.Fatal(err)
	}
	original, err := domain.ApplySequence(after, inverse)
	if err != nil || !reflect.DeepEqual(original, before) {
		t.Fatal(original, err)
	}
}
