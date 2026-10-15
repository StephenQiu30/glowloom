package prompt_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func editorDraft(t *testing.T) domain.Draft {
	t.Helper()
	d, err := domain.DecodeDraft([]byte(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"画一只猫"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestEditorDraftStrictAndHashes(t *testing.T) {
	d := editorDraft(t)
	original, input := d.Hashes()
	d.LabelState.HighWater["image"] = 1
	d.LabelState.Allocations["deleted"] = domain.LabelAllocation{LabelKind: "image", Ordinal: 1}
	changed, sameInput := d.Hashes()
	if original == changed || input != sameInput {
		t.Fatal("tombstone must save without changing generation intent")
	}
	if err := d.ExtendsLabels(editorDraft(t).LabelState); err != nil {
		t.Fatal(err)
	}
	if err := editorDraft(t).ExtendsLabels(d.LabelState); !errors.Is(err, domain.ErrLabelConflict) {
		t.Fatal("label history was reclaimed", err)
	}
	raw, _ := json.Marshal(d)
	var object map[string]any
	_ = json.Unmarshal(raw, &object)
	object["private_url"] = "https://invalid.test"
	bad, _ := json.Marshal(object)
	if _, err := domain.DecodeDraft(bad); err == nil {
		t.Fatal("unknown field accepted")
	}
	object["schemaVersion"] = 99
	delete(object, "private_url")
	bad, _ = json.Marshal(object)
	if _, err := domain.DecodeDraft(bad); !errors.Is(err, domain.ErrUnsupportedSchema) {
		t.Fatal(err)
	}
}
func TestEditorDraftReferenceCompilation(t *testing.T) {
	d := editorDraft(t)
	d.References = []domain.Reference{{ID: "ref", BindingGeneration: 1, MediaKind: "image", Role: "reference_image", Order: 0, Source: domain.ReferenceSource{Kind: "media", SourceID: "asset", SourceVersion: "1"}}}
	d.LabelState.HighWater["image"] = 1
	d.LabelState.Allocations["ref"] = domain.LabelAllocation{LabelKind: "image", Ordinal: 1}
	d.Document.Content[0].Content = []domain.Inline{{Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: "ref"}}, {Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: "ref"}}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	compiled, err := d.Compile()
	if err != nil || len(compiled.Media) != 1 || compiled.Prompt != "[图片1][图片1]" {
		t.Fatalf("compile: %+v %v", compiled, err)
	}
	d.References[0].Source = domain.ReferenceSource{Kind: "pending_upload", ClientUploadID: "file", UploadAttemptID: "try"}
	if err := d.Validate(); err != nil {
		t.Fatal("pending must be saveable", err)
	}
	if _, err := d.Compile(); !errors.Is(err, domain.ErrReferenceUnavailable) {
		t.Fatal("pending must block generation", err)
	}
	d.Document.Content[0].Content[0].Attrs.ReferenceID = "missing"
	if err := d.Validate(); err == nil {
		t.Fatal("dangling reference accepted")
	}
}

func TestReferenceSourceRejectsFieldsFromOtherVariants(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"pending_upload","clientUploadId":"f","uploadAttemptId":"a","sourceId":""}`,
		`{"kind":"media","sourceId":"m","sourceVersion":"1","clientUploadId":""}`,
		`{"kind":"subject","sourceId":"s","sourceVersion":"1","description":"","media":[],"uploadSessionId":""}`,
	} {
		var source domain.ReferenceSource
		if json.Unmarshal([]byte(raw), &source) == nil {
			t.Errorf("accepted mixed source: %s", raw)
		}
	}
}
