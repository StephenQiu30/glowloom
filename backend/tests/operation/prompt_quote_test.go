package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	promptapp "github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

type quotePreferenceReader struct{ calls int }

func (r *quotePreferenceReader) ReadCustomizations(context.Context, identityapp.Principal) ([]promptdomain.Customization, error) {
	r.calls++
	return nil, nil
}

func templateQuoteFixture(t *testing.T, operation string) (operationapp.CreateFreeQuoteInput, operationapp.FreeQuoteCatalog, promptapp.Preparation) {
	t.Helper()
	definition, _ := promptdomain.DefinitionFor(operation)
	revision := int64(0)
	request := &promptapp.TemplateRequest{Operation: operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision}
	input := operationapp.CreateFreeQuoteInput{
		ProjectID: uuid.New(), RequestID: uuid.NewString(),
		FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: "template-test", Capability: "text.structured", Mode: "structured", Prompt: "保留这次输入", OutputCount: 1, PromptTemplate: request},
	}
	if operation == "storyboard_video" {
		input.Capability, input.Mode = "video.generate", "text_to_video"
	}
	reader := &quotePreferenceReader{}
	prepared, err := promptapp.NewCompiler(reader).Prepare(t.Context(), identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: "producer"}, *request, promptapp.RuntimeContext{
		ProjectID: input.ProjectID, Capability: input.Capability, Mode: input.Mode, UserPrompt: input.Prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if operation == "storyboard_video" && reader.calls != 0 {
		t.Fatal("video quote read personal preferences")
	}
	catalog := operationapp.FreeQuoteCatalog{
		ModelVersionID: uuid.New(), ProviderModelID: "template-test-v1", Region: "domestic", Modes: []string{input.Mode},
		Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[]`),
		Price: catalogdomain.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: catalogdomain.PricePer1KChars, Rule: json.RawMessage(`{"base_micros":1000}`), Currency: "CNY", EffectiveFrom: time.Now().Add(-time.Minute)},
	}
	return input, catalog, prepared
}

func TestPrepareFreeQuoteWithTemplateFreezesFinalPromptAndPrice(t *testing.T) {
	input, catalog, compilation := templateQuoteFixture(t, "skill_draft")
	now := time.Now().UTC()
	quote, err := operationapp.PrepareFreeQuoteWithTemplate(input, catalog, nil, now, nil, &compilation)
	if err != nil {
		t.Fatal(err)
	}
	if len(quote.Inputs) != 1 || *quote.Inputs[0].TextValue != compilation.FinalPrompt || input.Prompt != compilation.OriginalPrompt || quote.PromptPreparation == nil {
		t.Fatal("quote did not freeze the compiled prompt independently from original input")
	}
	characters := int64(utf8.RuneCountInString(compilation.FinalPrompt))
	amount := ((characters + 999) / 1000) * 1000
	if *quote.Operation.QuoteMicros != amount || quote.PromptPreparation.ContentSHA256 != compilation.ContentSHA256 {
		t.Fatal("quote charged or froze the original prompt instead of final text")
	}
	plain := input
	plain.PromptTemplate, plain.Prompt = nil, compilation.FinalPrompt
	finalQuote, err := operationapp.PrepareFreeQuote(plain, catalog, nil, now, nil)
	if err != nil || finalQuote.Operation.InputHash != quote.Operation.InputHash {
		t.Fatal("compiled prompt did not participate in ordinary generation fingerprint", err)
	}
	plain.Prompt = compilation.OriginalPrompt
	originalQuote, err := operationapp.PrepareFreeQuote(plain, catalog, nil, now, nil)
	if err != nil || originalQuote.Operation.InputHash == quote.Operation.InputHash {
		t.Fatal("original prompt reused compiled prompt fingerprint", err)
	}
	if err := quote.PromptPreparation.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := operationapp.PrepareFreeQuote(input, catalog, nil, now, nil); !errors.Is(err, promptapp.ErrTemplateUnavailable) {
		t.Fatal("unwired legacy path silently ignored template selection", err)
	}
}

func TestPrepareFreeQuoteWithTemplateRejectsUnboundPreparation(t *testing.T) {
	input, catalog, compilation := templateQuoteFixture(t, "skill_draft")
	for _, test := range []struct {
		name   string
		change func(*promptapp.Preparation)
	}{
		{"wrong project", func(p *promptapp.Preparation) { p.ProjectID = uuid.New() }},
		{"wrong original", func(p *promptapp.Preparation) { p.OriginalPrompt += "别的输入" }},
		{"changed final text", func(p *promptapp.Preparation) { p.FinalPrompt += "别的输出" }},
		{"wrong operation", func(p *promptapp.Preparation) { p.Operation = "character_extract" }},
		{"wrong selection", func(p *promptapp.Preparation) { p.RequestSHA256 = strings.Repeat("b", 64) }},
		{"wrong baseline", func(p *promptapp.Preparation) { p.TemplateID = uuid.New() }},
		{"wrong personal revision", func(p *promptapp.Preparation) { p.CustomizationRevision = 2 }},
		{"wrong policy", func(p *promptapp.Preparation) { p.Policy = "bypass_video" }},
		{"missing template version", func(p *promptapp.Preparation) { p.TemplateVersion = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			altered := compilation
			test.change(&altered)
			if _, err := operationapp.PrepareFreeQuoteWithTemplate(input, catalog, nil, time.Now(), nil, &altered); !errors.Is(err, promptapp.ErrTemplateInvalid) {
				t.Fatal("unbound compilation accepted", err)
			}
		})
	}
	if _, err := operationapp.PrepareFreeQuoteWithTemplate(input, catalog, nil, time.Now(), nil, nil); !errors.Is(err, promptapp.ErrTemplateUnavailable) {
		t.Fatal("missing compilation silently ignored selected policy", err)
	}
}

func TestPrepareFreeQuoteWithTemplatePreservesPlainAndVideoPaths(t *testing.T) {
	input, catalog, compilation := templateQuoteFixture(t, "storyboard_video")
	now := time.Now()
	videoQuote, err := operationapp.PrepareFreeQuoteWithTemplate(input, catalog, nil, now, nil, &compilation)
	if err != nil || *videoQuote.Inputs[0].TextValue != input.Prompt || videoQuote.PromptPreparation.Policy != "bypass_video" || videoQuote.PromptPreparation.TemplateID != nil {
		t.Fatal("source video bypass changed the prompt or froze a template", err)
	}
	plain := input
	plain.PromptTemplate = nil
	legacyQuote, err := operationapp.PrepareFreeQuote(plain, catalog, nil, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := operationapp.PrepareFreeQuoteWithTemplate(plain, catalog, nil, now, nil, nil)
	if err != nil || quote.PromptPreparation != nil || quote.Operation.InputHash != legacyQuote.Operation.InputHash || *quote.Operation.QuoteMicros != *legacyQuote.Operation.QuoteMicros {
		t.Fatal("plain request changed its quote or fingerprint", err)
	}
	if _, err := operationapp.PrepareFreeQuoteWithTemplate(plain, catalog, nil, now, nil, &compilation); !errors.Is(err, promptapp.ErrTemplateInvalid) {
		t.Fatal("unselected compilation changed a plain request", err)
	}
}

func TestFrozenPromptPreparationRejectsIncompleteEvidence(t *testing.T) {
	input, catalog, compilation := templateQuoteFixture(t, "skill_draft")
	quote, err := operationapp.PrepareFreeQuoteWithTemplate(input, catalog, nil, time.Now(), nil, &compilation)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*operationdomain.PromptPreparation)
	}{
		{"unsupported version", func(p *operationdomain.PromptPreparation) { p.Version++ }},
		{"unsupported operation", func(p *operationdomain.PromptPreparation) { p.Operation = "arbitrary" }},
		{"invalid hash", func(p *operationdomain.PromptPreparation) { p.ContentSHA256 = "invalid" }},
		{"missing baseline", func(p *operationdomain.PromptPreparation) { p.TemplateID = nil }},
		{"revision without customization", func(p *operationdomain.PromptPreparation) { p.CustomizationRevision = 2 }},
		{"customization without revision", func(p *operationdomain.PromptPreparation) { id := uuid.New(); p.CustomizationID = &id }},
	} {
		t.Run(test.name, func(t *testing.T) {
			altered := *quote.PromptPreparation
			test.change(&altered)
			if err := altered.Validate(); !errors.Is(err, operationdomain.ErrInvalidPromptPreparation) {
				t.Fatal("incomplete frozen prompt evidence accepted", err)
			}
		})
	}
}
