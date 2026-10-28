package prompt_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"

	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type subjectSourceOwner struct {
	source                 domain.ReferenceSource
	resolveErr, bindingErr error
	calls                  int
}

func (o *subjectSourceOwner) ResolveSubject(context.Context, identity.Principal, uuid.UUID, string, string) (domain.ReferenceSource, bool, error) {
	return o.source, true, o.resolveErr
}
func (o *subjectSourceOwner) ValidateBinding(context.Context, identity.Principal, uuid.UUID, domain.SourceMedia, bool) error {
	o.calls++
	return o.bindingErr
}

func TestUnifiedSubjectDraftRequiresOwningBindingProof(t *testing.T) {
	d := editorDraft(t)
	source := domain.ReferenceSource{Kind: "subject", SourceID: uuid.NewString(), SourceVersion: uuid.NewString(), Description: "主体", Media: []domain.SourceMedia{{SourceID: uuid.NewString(), SourceVersion: "1", MediaKind: "image", BindingID: uuid.NewString()}}}
	d.References = []domain.Reference{{ID: "ref", Source: source, BindingGeneration: 1, MediaKind: "image", Role: "subject", Order: 0}}
	d.LabelState.HighWater["subject"] = 1
	d.LabelState.Allocations["ref"] = domain.LabelAllocation{LabelKind: "subject", Ordinal: 1}
	tx := &gorm.DB{Statement: &gorm.Statement{Context: t.Context()}}
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New()}
	project := uuid.New()
	if err := promptpg.ValidateDraftSources(tx, actor, project, d, false); !errors.Is(err, domain.ErrReferenceUnavailable) {
		t.Fatalf("missing owner did not fail closed: %v", err)
	}
	owner := &subjectSourceOwner{source: source}
	if err := promptpg.ValidateDraftSources(tx, actor, project, d, true, owner); err != nil || owner.calls != 1 {
		t.Fatalf("binding owner not checked: %v, %d", err, owner.calls)
	}
	owner.bindingErr = domain.ErrReferenceUnavailable
	if err := promptpg.ValidateDraftSources(tx, actor, project, d, true, owner); !errors.Is(err, domain.ErrReferenceUnavailable) {
		t.Fatalf("revoked binding accepted: %v", err)
	}
	owner.bindingErr = nil
	owner.resolveErr = domain.ErrReferenceUnavailable
	if err := promptpg.ValidateDraftSources(tx, actor, project, d, false, owner); !errors.Is(err, domain.ErrReferenceUnavailable) {
		t.Fatalf("subject access failure fell back to legacy: %v", err)
	}
}

func TestSubjectCompilationRetainsDistinctBindingProofs(t *testing.T) {
	d := editorDraft(t)
	d.References = []domain.Reference{
		{ID: "one", BindingGeneration: 1, MediaKind: "image", Role: "subject", Order: 0, Source: domain.ReferenceSource{Kind: "subject", SourceID: "subject-one", SourceVersion: "v1", Media: []domain.SourceMedia{{SourceID: "asset", SourceVersion: "1", MediaKind: "image", BindingID: "binding-one"}}}},
		{ID: "two", BindingGeneration: 1, MediaKind: "image", Role: "subject", Order: 1, Source: domain.ReferenceSource{Kind: "subject", SourceID: "subject-two", SourceVersion: "v2", Media: []domain.SourceMedia{{SourceID: "asset", SourceVersion: "1", MediaKind: "image", BindingID: "binding-two"}}}},
	}
	d.LabelState.HighWater["subject"] = 2
	d.LabelState.Allocations["one"] = domain.LabelAllocation{LabelKind: "subject", Ordinal: 1}
	d.LabelState.Allocations["two"] = domain.LabelAllocation{LabelKind: "subject", Ordinal: 2}
	d.Document.Content[0].Content = []domain.Inline{{Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: "one"}}, {Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: "two"}}}
	compiled, err := d.Compile()
	if err != nil || len(compiled.Media) != 2 || compiled.Media[0].BindingID != "binding-one" || compiled.Media[1].BindingID != "binding-two" {
		t.Fatalf("subject proof lost during compilation: %+v, %v", compiled, err)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := domain.DecodeDraft(raw)
	if err != nil || roundTrip.References[1].Source.Media[0].BindingID != "binding-two" {
		t.Fatalf("subject proof lost during round trip: %+v, %v", roundTrip, err)
	}
}

func TestUnifiedSubjectExpansionUsesEachPhysicalMediaRole(t *testing.T) {
	d := editorDraft(t)
	source := domain.ReferenceSource{Kind: "subject", SourceID: "new-subject", SourceVersion: "new-version", Description: "主体", Media: []domain.SourceMedia{}}
	for _, kind := range []string{"image", "video", "audio"} {
		source.Media = append(source.Media, domain.SourceMedia{SourceID: kind, SourceVersion: "1", MediaKind: kind, BindingID: "binding-" + kind})
	}
	d.References = []domain.Reference{{ID: "subject", BindingGeneration: 1, MediaKind: "image", Role: "subject", Source: source}}
	d.LabelState.HighWater["subject"] = 1
	d.LabelState.Allocations["subject"] = domain.LabelAllocation{LabelKind: "subject", Ordinal: 1}
	d.Document.Content[0].Content = []domain.Inline{{Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: "subject"}}}
	compiled, err := d.Compile()
	if err != nil || len(compiled.Media) != 3 {
		t.Fatalf("expand: %+v %v", compiled, err)
	}
	for index, kind := range []string{"image", "video", "audio"} {
		if compiled.Media[index].Role != "reference_"+kind || compiled.Media[index].BindingID != "binding-"+kind {
			t.Fatalf("physical %s role: %+v", kind, compiled.Media[index])
		}
	}
	if compiled.Mapping[0].Role != "subject" {
		t.Fatal("semantic subject mapping lost")
	}
	// Old Bible snapshots retain their historical role and fingerprint rules.
	for index := range d.References[0].Source.Media {
		d.References[0].Source.Media[index].BindingID = ""
	}
	legacy, err := d.Compile()
	if err != nil || legacy.Media[0].Role != "subject" {
		t.Fatalf("legacy role changed: %+v %v", legacy, err)
	}
}
