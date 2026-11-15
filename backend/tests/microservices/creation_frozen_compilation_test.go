package microservices_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func compilationEnvelope(t *testing.T) (prompt.FrozenInputEnvelope, prompt.CompileFrozenInputRequest) {
	t.Helper()
	identity, admission := sourceAdmissionFixture()
	admission.Command.Action = "execution.frozen_input.accept"
	model, mode := uuid.New(), "text"
	modelString := model.String()
	draft := frozenDraft("immutable original request")
	draft.ModelVersion, draft.Mode = &modelString, &mode
	draft.Params["output_count"], draft.Params["temperature"] = float64(3), float64(0.5)
	input := domain.FrozenInput{HandoffID: uuid.New(), RequestID: uuid.New(), Command: admission.Command, ActorID: identity.AccountID, SessionID: identity.SessionID, ScopeID: admission.ScopeID, ProjectID: admission.ProjectID, OrgID: uuid.New(), AspectRatio: "16:9", Source: domain.InputSource{Kind: "prompt_draft", DraftID: uuid.New(), Revision: 1}, SourceGeneration: 1, DraftRevision: 1, ScopeRevision: 1, ProjectRevision: 1, Draft: draft, Pins: []domain.FrozenPin{}, CompilationInputs: &domain.FrozenCompilationInputs{CompilerVersion: domain.CompilerVersion, ProjectName: "原项目", StyleType: "realistic"}}
	input.SourceCommand = writefence.Command{ID: uuid.NewSHA1(input.HandoffID, []byte("creation.frozen_input.freeze")), SHA256: strings.Repeat("b", 64), Action: "creation.frozen_input.freeze", Authorities: append([]writefence.Authority(nil), admission.Command.Authorities...)}
	input.DraftHash, input.InputHash = draft.Hashes()
	envelope := prompt.FrozenInputEnvelope{Input: input}
	resealCompilationEnvelope(t, &envelope)
	return envelope, prompt.CompileFrozenInputRequest{Key: envelope.Key(), CatalogContext: prompt.FrozenCatalogContext{ModelVersionID: model, ModelKey: "original-model", Capability: "text.structured", Mode: mode}}
}

func resealCompilationEnvelope(t *testing.T, envelope *prompt.FrozenInputEnvelope) {
	t.Helper()
	envelope.Input.DraftHash, envelope.Input.InputHash = envelope.Input.Draft.Hashes()
	_, hash, err := envelope.Input.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	envelope.SnapshotSHA256 = hash
}

func TestCreationFrozenCompilationReusesActualDraftCompilerAndContext(t *testing.T) {
	envelope, request := compilationEnvelope(t)
	actual, err := prompt.CompileFrozenSnapshot(envelope, request)
	if err != nil {
		t.Fatal(err)
	}
	original, err := envelope.Input.Draft.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if actual.OriginalPrompt != original.Prompt || actual.FinalPrompt != original.Prompt || actual.OutputCount != 3 || actual.CompilerVersion != domain.CompilerVersion || len(actual.CompilationSHA256) != 64 {
		t.Fatalf("compiler output drift: %+v", actual)
	}
	var params map[string]any
	if json.Unmarshal(actual.ParamsJSON, &params) != nil || len(params) != 1 || params["temperature"] != 0.5 {
		t.Fatal("output_count leaked into Catalog parameters", string(actual.ParamsJSON))
	}
	repeat, err := prompt.CompileFrozenSnapshot(envelope, request)
	if err != nil || !reflect.DeepEqual(actual, repeat) {
		t.Fatal("deterministic result drift", err)
	}
	first, _ := prompt.FrozenCompilationCommand(envelope, request)
	changed := request
	changed.CatalogContext.ModelKey = "changed-model"
	second, err := prompt.FrozenCompilationCommand(envelope, changed)
	if err != nil || first.ID != second.ID || first.SHA256 == second.SHA256 {
		t.Fatal("different immutable context failed to conflict at original command ID", err)
	}
	if !reflect.DeepEqual(first.Authorities, envelope.Input.SourceCommand.Authorities) {
		t.Fatal("credential revision substituted for original authority epochs")
	}
}

func TestCreationFrozenCompilationPreservesMediaOrderAndMotionPrompt(t *testing.T) {
	envelope, request := compilationEnvelope(t)
	asset := uuid.New()
	for i, id := range []string{"first", "second"} {
		envelope.Input.Draft.References = append(envelope.Input.Draft.References, domain.Reference{ID: id, Source: domain.ReferenceSource{Kind: "media", SourceID: asset.String(), SourceVersion: "1"}, BindingGeneration: 1, MediaKind: "image", Role: "reference_image", Order: i})
		envelope.Input.Draft.LabelState.Allocations[id] = domain.LabelAllocation{LabelKind: "image", Ordinal: int64(i + 1)}
		envelope.Input.Pins = append(envelope.Input.Pins, domain.FrozenPin{ReferenceID: id, PinID: uuid.New(), AcquireCommandID: uuid.New(), AssetID: asset, AssetRevision: 1, Kind: "image", SnapshotSHA256: strings.Repeat("c", 64)})
		envelope.Input.Draft.Document.Content[0].Content = append(envelope.Input.Draft.Document.Content[0].Content, domain.Inline{Type: "reference", Attrs: &domain.ReferenceAttrs{ReferenceID: id}})
	}
	envelope.Input.Draft.LabelState.HighWater["image"] = 2
	resealCompilationEnvelope(t, &envelope)
	request.Key = envelope.Key()
	compiled, err := prompt.CompileFrozenSnapshot(envelope, request)
	if err != nil || len(compiled.Media) != 1 || compiled.Media[0].SeqNo != 1 || compiled.Media[0].MediaAssetID != asset || len(compiled.ReferenceMapping) != 2 {
		t.Fatal("physical media ordering/deduplication changed", compiled, err)
	}
	for i, mapping := range compiled.ReferenceMapping {
		if len(mapping.MediaSeqNos) != 1 || mapping.MediaSeqNos[0] != 1 || mapping.ReferenceID != []string{"first", "second"}[i] {
			t.Fatal("display mapping stopped using one-based physical positions", mapping)
		}
	}
	mode := "motion"
	envelope.Input.Draft.Mode, request.CatalogContext.Mode = &mode, mode
	resealCompilationEnvelope(t, &envelope)
	request.Key = envelope.Key()
	motion, err := prompt.CompileFrozenSnapshot(envelope, request)
	if err != nil || motion.FinalPrompt != "" || motion.OriginalPrompt != "" {
		t.Fatal("motion's intentional empty prompt was replaced", motion, err)
	}
}

func TestCreationFrozenCompilationTemplateUsesFrozenPersonalLayer(t *testing.T) {
	envelope, request := compilationEnvelope(t)
	definition, _ := domain.DefinitionFor("skill_draft")
	revision := int64(2)
	selected := prompt.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision}
	preference := domain.Customization{ID: uuid.New(), Operation: definition.Operation, Mode: domain.Append, Content: "冻结的个人写作约定", BaseTemplateID: definition.TemplateID, Revision: 2, UpdateTime: time.Now().UTC()}
	frozen, err := prompt.CaptureFrozenTemplate(selected, []domain.Customization{preference})
	if err != nil {
		t.Fatal(err)
	}
	envelope.Input.CompilationInputs.Template = &frozen
	resealCompilationEnvelope(t, &envelope)
	request.Key = envelope.Key()
	// Changing the caller's preference value cannot mutate the accepted snapshot.
	preference.Content = "后来偏好"
	actual, err := prompt.CompileFrozenSnapshot(envelope, request)
	if err != nil || !strings.Contains(actual.FinalPrompt, "冻结的个人写作约定") || strings.Contains(actual.FinalPrompt, "后来偏好") || actual.TemplateEvidence == nil || actual.TemplateEvidence.CustomizationRevision != 2 {
		t.Fatalf("frozen template drift %+v %v", actual, err)
	}
	if actual.TemplateEvidence.UserPromptSHA256 == actual.TemplateEvidence.ContentSHA256 {
		t.Fatal("original and compiled prompt evidence conflated")
	}
	if _, err := prompt.CaptureFrozenTemplate(selected, []domain.Customization{preference, preference}); !errors.Is(err, prompt.ErrTemplateInvalid) {
		t.Fatal("ambiguous preferences selected arbitrarily", err)
	}
}

func TestCreationFrozenCompilationFailsClosedAndPreservesVideoBypass(t *testing.T) {
	for _, kind := range []string{"missing-inputs", "unknown-compiler", "model-mismatch", "mode-mismatch", "fractional-count", "zero-count", "large-count", "blocked", "missing-protected-context"} {
		t.Run(kind, func(t *testing.T) {
			envelope, request := compilationEnvelope(t)
			switch kind {
			case "missing-inputs":
				envelope.Input.CompilationInputs = nil
			case "unknown-compiler":
				envelope.Input.CompilationInputs.CompilerVersion = "unsupported-compiler"
			case "model-mismatch":
				request.CatalogContext.ModelVersionID = uuid.New()
			case "mode-mismatch":
				request.CatalogContext.Mode = "video"
			case "fractional-count":
				envelope.Input.Draft.Params["output_count"] = 1.5
			case "zero-count":
				envelope.Input.Draft.Params["output_count"] = float64(0)
			case "large-count":
				envelope.Input.Draft.Params["output_count"] = float64(9)
			case "blocked":
				envelope.AdoptionBlocked = true
			case "missing-protected-context":
				definition, _ := domain.DefinitionFor("chapter_assets_extract")
				revision := int64(0)
				frozen, err := prompt.CaptureFrozenTemplate(prompt.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision}, nil)
				if err != nil {
					t.Fatal(err)
				}
				envelope.Input.CompilationInputs.Template = &frozen
			}
			resealCompilationEnvelope(t, &envelope)
			request.Key = envelope.Key()
			_, err := prompt.CompileFrozenSnapshot(envelope, request)
			if err == nil {
				t.Fatal("invalid compilation accepted")
			}
			if kind == "missing-protected-context" && !errors.Is(err, prompt.ErrTemplateContextUnavailable) {
				t.Fatal("missing authoritative context fell back", err)
			}
		})
	}
	envelope, request := compilationEnvelope(t)
	definition, _ := domain.DefinitionFor("storyboard_video")
	revision := int64(99)
	frozen, err := prompt.CaptureFrozenTemplate(prompt.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision}, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Input.CompilationInputs.Template = &frozen
	request.CatalogContext.Capability = "video.generate"
	resealCompilationEnvelope(t, &envelope)
	request.Key = envelope.Key()
	actual, err := prompt.CompileFrozenSnapshot(envelope, request)
	if err != nil || actual.OriginalPrompt != actual.FinalPrompt || actual.TemplateEvidence == nil || actual.TemplateEvidence.Policy != "bypass_video" || actual.TemplateEvidence.TemplateID != nil {
		t.Fatal("video bypass rewrote original text or applied preference", actual, err)
	}
}
