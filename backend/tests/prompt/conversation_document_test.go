package prompt_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestConversationDocumentReferencePreservesIdentityAndLegacyLabels(t *testing.T) {
	d := editorDraft(t)
	d.LabelState.HighWater["document"] = 1
	d.References = []domain.Reference{{ID: "novel", Source: domain.ReferenceSource{Kind: "media", SourceID: "00000000-0000-4000-8000-000000000001", SourceVersion: "7"}, BindingGeneration: 1, MediaKind: "document", Role: "reference_document", Order: 0}}
	d.LabelState.Allocations["novel"] = domain.LabelAllocation{LabelKind: "document", Ordinal: 1}
	in, err := domain.NewConversationInput(d)
	if err != nil || len(in.MediaPins) != 1 || in.MediaPins[0].MediaKind != "document" || in.MediaPins[0].SourceVersion != "7" {
		t.Fatalf("document input=%+v error=%v", in, err)
	}
	if _, err := domain.NewConversationInput(editorDraft(t)); err != nil {
		t.Fatal("legacy input rejected", err)
	}
}
