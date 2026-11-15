package application

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FrozenCatalogContext contains only Execution's immutable selected Catalog facts.
// Schema, limits, pricing and release policy remain at their Execution owner.
type FrozenCatalogContext struct {
	ModelVersionID uuid.UUID `json:"model_version_id"`
	ModelKey       string    `json:"model_key"`
	Capability     string    `json:"capability"`
	Mode           string    `json:"mode"`
}

// SHA256 binds the complete immutable context without accepting caller defaults.
func (c FrozenCatalogContext) SHA256() (string, error) {
	if c.ModelVersionID == uuid.Nil {
		return "", domain.ErrFrozenInputInvalid
	}
	for _, value := range []string{c.ModelKey, c.Capability, c.Mode} {
		if strings.TrimSpace(value) != value || value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return "", domain.ErrFrozenInputInvalid
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", domain.ErrFrozenInputInvalid
	}
	return contentHash(string(raw)), nil
}

// CompileFrozenInputRequest references the original snapshot and immutable Catalog.
type CompileFrozenInputRequest struct {
	Key            FrozenInputKey       `json:"key"`
	CatalogContext FrozenCatalogContext `json:"catalog_context"`
}

// FrozenCompiledMedia retains the compiler's exact one-based physical ordering.
type FrozenCompiledMedia struct {
	SeqNo                int32      `json:"seq_no"`
	MediaAssetID         uuid.UUID  `json:"media_asset_id"`
	Role                 string     `json:"role"`
	SubjectBindingID     *uuid.UUID `json:"subject_binding_id,omitempty"`
	SubjectBindingSHA256 string     `json:"subject_binding_sha256,omitempty"`
}

// FrozenTemplateEvidence is the original quote's bounded prompt evidence.
type FrozenTemplateEvidence struct {
	Version               int        `json:"version"`
	Operation             string     `json:"operation"`
	Policy                string     `json:"policy"`
	TemplateID            *uuid.UUID `json:"template_id,omitempty"`
	TemplateVersion       int        `json:"template_version,omitempty"`
	CustomizationID       *uuid.UUID `json:"customization_id,omitempty"`
	CustomizationRevision int64      `json:"customization_revision,omitempty"`
	RequestSHA256         string     `json:"request_sha256"`
	UserPromptSHA256      string     `json:"user_prompt_sha256"`
	ContentSHA256         string     `json:"content_sha256"`
}

// FrozenInputCompilation is persisted whole in the original compile command's
// receipt. It grants neither execution, billing permission nor source adoption.
type FrozenInputCompilation struct {
	Key               FrozenInputKey            `json:"key"`
	CatalogContext    FrozenCatalogContext      `json:"catalog_context"`
	ContextSHA256     string                    `json:"context_sha256"`
	CompilationSHA256 string                    `json:"compilation_sha256"`
	ModelVersionID    uuid.UUID                 `json:"model_version_id"`
	Mode              string                    `json:"mode"`
	OriginalPrompt    string                    `json:"original_prompt"`
	FinalPrompt       string                    `json:"final_prompt"`
	ParamsJSON        json.RawMessage           `json:"params_json"`
	OutputCount       int32                     `json:"output_count"`
	Media             []FrozenCompiledMedia     `json:"media"`
	CompilerVersion   string                    `json:"compiler_version"`
	ReferenceMapping  []domain.ReferenceMapping `json:"reference_mapping"`
	TemplateEvidence  *FrozenTemplateEvidence   `json:"template_evidence,omitempty"`
}

// Receipt canonicalizes all result fields; the compilation digest omits only itself.
func (c FrozenInputCompilation) Receipt() (json.RawMessage, error) {
	expected := c.CompilationSHA256
	c.CompilationSHA256 = ""
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, domain.ErrFrozenInputInvalid
	}
	canonical, err := writefence.CanonicalReceipt(raw)
	if err != nil {
		return nil, err
	}
	c.CompilationSHA256 = contentHash(string(canonical))
	if expected != "" && expected != c.CompilationSHA256 {
		return nil, domain.ErrFrozenInputConflict
	}
	raw, err = json.Marshal(c)
	if err != nil {
		return nil, domain.ErrFrozenInputInvalid
	}
	return writefence.CanonicalReceipt(raw)
}

// FrozenCompilationCommand binds the separate Creation-local effect to the
// original consumer key/context and exactly the original four source authorities.
func FrozenCompilationCommand(e FrozenInputEnvelope, request CompileFrozenInputRequest) (writefence.Command, error) {
	if e.Key() != request.Key {
		return writefence.Command{}, domain.ErrFrozenInputConflict
	}
	if _, err := request.CatalogContext.SHA256(); err != nil {
		return writefence.Command{}, err
	}
	if e.Input.Validate() != nil {
		return writefence.Command{}, domain.ErrFrozenInputInvalid
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return writefence.Command{}, domain.ErrFrozenInputInvalid
	}
	return writefence.Command{ID: uuid.NewSHA1(e.Input.HandoffID, []byte("creation.frozen_input.compile")), SHA256: contentHash("glowloom.creation.compile\x00" + string(raw)), Action: "creation.frozen_input.compile", Authorities: append([]writefence.Authority(nil), e.Input.SourceCommand.Authorities...)}, nil
}

// CompileFrozenSnapshot is pure source business logic. It uses the same Draft
// and template compilers as the original quote and cannot load mutable facts.
func CompileFrozenSnapshot(e FrozenInputEnvelope, request CompileFrozenInputRequest) (FrozenInputCompilation, error) {
	var out FrozenInputCompilation
	if e.Key() != request.Key {
		return out, domain.ErrFrozenInputConflict
	}
	if e.AdoptionBlocked {
		return out, domain.ErrFrozenInputBlocked
	}
	_, hash, err := e.Input.Snapshot()
	if err != nil || hash != e.SnapshotSHA256 {
		return out, domain.ErrFrozenInputConflict
	}
	inputs := e.Input.CompilationInputs
	if inputs == nil || inputs.Validate() != nil || inputs.CompilerVersion != domain.CompilerVersion {
		return out, domain.ErrUnsupportedSchema
	}
	contextHash, err := request.CatalogContext.SHA256()
	if err != nil {
		return out, err
	}
	draft, catalog := e.Input.Draft, request.CatalogContext
	if draft.ModelVersion == nil || *draft.ModelVersion != catalog.ModelVersionID.String() || draft.Mode == nil || *draft.Mode != catalog.Mode {
		return out, domain.ErrInputStale
	}
	if draft.CinemaKit != nil && !draft.CinemaKit.Supports(catalog.Capability) {
		return out, domain.ErrInvalidDraft
	}
	compiled, err := draft.Compile()
	if err != nil {
		return out, err
	}
	params := map[string]any{}
	for key, value := range draft.Params {
		if key != "output_count" {
			params[key] = value
		}
	}
	count := float64(1)
	if value, exists := draft.Params["output_count"]; exists {
		var valid bool
		count, valid = value.(float64)
		if !valid || math.Trunc(count) != count || count < 1 || count > 8 {
			return out, domain.ErrInvalidDraft
		}
	}
	paramJSON, err := json.Marshal(params)
	if err != nil {
		return out, domain.ErrInvalidDraft
	}
	out = FrozenInputCompilation{Key: request.Key, CatalogContext: catalog, ContextSHA256: contextHash, ModelVersionID: catalog.ModelVersionID, Mode: catalog.Mode, OriginalPrompt: compiled.Prompt, FinalPrompt: compiled.Prompt, ParamsJSON: paramJSON, OutputCount: int32(count), Media: []FrozenCompiledMedia{}, CompilerVersion: inputs.CompilerVersion, ReferenceMapping: compiled.Mapping}
	if out.ReferenceMapping == nil {
		out.ReferenceMapping = []domain.ReferenceMapping{}
	}
	for i, media := range compiled.Media {
		id, err := uuid.Parse(media.SourceID)
		if err != nil || id == uuid.Nil {
			return FrozenInputCompilation{}, domain.ErrReferenceUnavailable
		}
		item := FrozenCompiledMedia{SeqNo: int32(i + 1), MediaAssetID: id, Role: media.Role}
		if media.BindingID != "" {
			binding, err := uuid.Parse(media.BindingID)
			if err != nil || binding == uuid.Nil {
				return FrozenInputCompilation{}, domain.ErrReferenceUnavailable
			}
			item.SubjectBindingID, item.SubjectBindingSHA256 = &binding, media.SourceVersion
		}
		out.Media = append(out.Media, item)
	}
	if inputs.Template != nil {
		prepared, err := prepareFrozenTemplate(*inputs, RuntimeContext{ProjectID: e.Input.ProjectID, Capability: catalog.Capability, Mode: catalog.Mode, UserPrompt: compiled.Prompt})
		if err != nil {
			return FrozenInputCompilation{}, err
		}
		out.FinalPrompt = prepared.FinalPrompt
		evidence := &FrozenTemplateEvidence{Version: 1, Operation: prepared.Operation, Policy: prepared.Policy, RequestSHA256: prepared.RequestSHA256, UserPromptSHA256: prepared.UserPromptSHA256, ContentSHA256: prepared.ContentSHA256}
		if prepared.TemplateID != uuid.Nil {
			evidence.TemplateID, evidence.TemplateVersion = &prepared.TemplateID, prepared.TemplateVersion
		}
		if prepared.CustomizationID != uuid.Nil {
			evidence.CustomizationID, evidence.CustomizationRevision = &prepared.CustomizationID, prepared.CustomizationRevision
		}
		out.TemplateEvidence = evidence
	}
	raw, err := out.Receipt()
	if err != nil {
		return FrozenInputCompilation{}, err
	}
	if json.Unmarshal(raw, &out) != nil {
		return FrozenInputCompilation{}, domain.ErrFrozenInputInvalid
	}
	return out, nil
}

// FrozenCompilationStore owns the final local writer Guard/result/Outbox transaction.
type FrozenCompilationStore interface {
	ReadFrozenCompilation(context.Context, CompileFrozenInputRequest) (FrozenInputCompilation, error)
	CompileFrozenInput(context.Context, CompileFrozenInputRequest, writefence.Command) (FrozenInputCompilation, error)
}
