package domain

import (
	"strings"
	"testing"
)

func conversationDraft(t *testing.T, text string) Draft {
	t.Helper()
	d, err := DecodeDraft([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph"}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		d.Document.Content[0].Content = []Inline{{Type: "text", Text: text}}
	}
	return d
}
func TestConversationInputPreservesNewTextAndRejectsEmpty(t *testing.T) {
	d := conversationDraft(t, "  为这幅图写一个故事\n  ")
	input, err := NewConversationInput(d)
	if err != nil || input.PlainText != "  为这幅图写一个故事\n  " || input.InputHash == "" {
		t.Fatalf("input=%+v err=%v", input, err)
	}
	if _, err := NewConversationInput(conversationDraft(t, " \n ")); err == nil {
		t.Fatal("empty input accepted")
	}
}
func TestConversationImageOnlyDoesNotInventTypedText(t *testing.T) {
	d := conversationDraft(t, "")
	d.References = []Reference{{ID: "ref", Source: ReferenceSource{Kind: "media", SourceID: "00000000-0000-4000-8000-000000000001", SourceVersion: "2"}, BindingGeneration: 1, MediaKind: "image", Role: "reference_image", Order: 0}}
	d.LabelState.HighWater["image"] = 1
	d.LabelState.Allocations["ref"] = LabelAllocation{LabelKind: "image", Ordinal: 1}
	input, err := NewConversationInput(d)
	if err != nil || input.PlainText != "" || len(input.MediaPins) != 1 || input.MediaPins[0].SourceVersion != "2" {
		t.Fatalf("image-only input=%+v err=%v", input, err)
	}
	before := input.InputHash
	d.References[0].Source.SourceVersion = "3"
	next, err := NewConversationInput(d)
	if err != nil || next.InputHash == before {
		t.Fatal("reference version was not frozen")
	}
	d.References[0].Source = ReferenceSource{Kind: "pending_upload", ClientUploadID: "client", UploadAttemptID: "attempt"}
	if _, err := NewConversationInput(d); err == nil {
		t.Fatal("pending upload accepted")
	}
}
func TestConversationInputBoundsTheWholeDraft(t *testing.T) {
	d := conversationDraft(t, strings.Repeat("图", 400000))
	if _, err := NewConversationInput(d); err == nil {
		t.Fatal("oversized input accepted")
	}
}
