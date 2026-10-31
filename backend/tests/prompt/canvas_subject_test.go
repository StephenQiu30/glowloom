package prompt_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestCanvasSubjectDraftCompilation(t *testing.T) {
	d := editorDraft(t)
	d.References = []domain.Reference{{ID: "local", BindingGeneration: 1, MediaKind: "image", Role: "subject", Order: 0, Source: domain.ReferenceSource{Kind: "canvas_subject", SourceID: "canvas-node", SourceVersion: "4", Description: strings.Repeat("局部说明", 2000), Media: []domain.SourceMedia{{SourceID: "image", SourceVersion: "2", MediaKind: "image"}, {SourceID: "audio", SourceVersion: "1", MediaKind: "audio"}}}}}
	d.LabelState.HighWater["subject"] = 1
	d.LabelState.Allocations["local"] = domain.LabelAllocation{LabelKind: "subject", Ordinal: 1}
	d.Document.Content[0].Content = []domain.Inline{{Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: "local"}}}
	encoded, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeDraft(encoded)
	if err != nil {
		t.Fatal("saved source roundtrip", err)
	}
	compiled, err := decoded.Compile()
	if err != nil || len(compiled.Media) != 2 || compiled.Media[0].Role != "reference_image" || compiled.Media[1].Role != "reference_audio" || !strings.Contains(compiled.Prompt, "[主体1]") {
		t.Fatal("physical subject inputs", compiled, err)
	}
	d.References[0].Source.Kind = "subject"
	if d.Validate() == nil {
		t.Fatal("canvas limit relaxed library source")
	}
	var invalid domain.ReferenceSource
	if json.Unmarshal([]byte(`{"kind":"canvas_subject","sourceId":"n","sourceVersion":"1","description":"","media":[],"subjectId":"library"}`), &invalid) == nil {
		t.Fatal("foreign identity accepted")
	}
}
