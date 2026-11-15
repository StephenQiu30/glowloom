package application

import (
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
)

// CaptureFrozenTemplate validates the explicit selection against the server-owned
// baseline and preferences read under the source transaction's preference lock.
func CaptureFrozenTemplate(request TemplateRequest, customizations []domain.Customization) (domain.FrozenTemplate, error) {
	if err := request.ValidateFor(templateCapability(request.Operation)); err != nil {
		return domain.FrozenTemplate{}, err
	}
	definition, _ := domain.DefinitionFor(request.Operation)
	if definition.TemplateID != request.ExpectedTemplateID {
		return domain.FrozenTemplate{}, ErrTemplateChanged
	}
	hash, err := request.Fingerprint()
	if err != nil {
		return domain.FrozenTemplate{}, err
	}
	out := domain.FrozenTemplate{Definition: definition, RequestSHA256: hash}
	if request.Outline != nil {
		outline := *request.Outline
		out.Outline = &outline
	}
	// Preserve the original source video bypass: it does not read or apply a
	// personal template layer and does not reinterpret the original user prompt.
	if request.Operation == "storyboard_video" {
		return out, nil
	}
	for _, customization := range customizations {
		if customization.Operation != request.Operation {
			continue
		}
		if out.Customization != nil || customization.ValidateAgainst(definition) != nil {
			return domain.FrozenTemplate{}, ErrTemplateInvalid
		}
		captured := customization
		out.Customization = &captured
	}
	revision := int64(0)
	if out.Customization != nil {
		revision = out.Customization.Revision
		if out.Customization.Mode == domain.Rewrite && out.Customization.BaseTemplateID != definition.TemplateID {
			return domain.FrozenTemplate{}, ErrTemplateChanged
		}
	}
	if revision != *request.ExpectedCustomizationRevision {
		return domain.FrozenTemplate{}, ErrTemplateChanged
	}
	return out, nil
}

func prepareFrozenTemplate(inputs domain.FrozenCompilationInputs, runtime RuntimeContext) (Preparation, error) {
	if inputs.Validate() != nil || inputs.Template == nil {
		return Preparation{}, ErrTemplateInvalid
	}
	template := inputs.Template
	definition := template.Definition
	if runtime.Capability != templateCapability(definition.Operation) || runtime.ProjectID == [16]byte{} || runtime.Mode == "" || !validQuotePrompt(runtime.UserPrompt) {
		return Preparation{}, ErrTemplateInvalid
	}
	prepared := Preparation{ProjectID: runtime.ProjectID, OriginalPrompt: runtime.UserPrompt, Operation: definition.Operation, RequestSHA256: template.RequestSHA256, UserPromptSHA256: contentHash(runtime.UserPrompt)}
	if definition.Operation == "storyboard_video" {
		prepared.FinalPrompt, prepared.Policy = runtime.UserPrompt, "bypass_video"
		prepared.ContentSHA256 = prepared.UserPromptSHA256
		return prepared, nil
	}
	// Only the immutable source project is available to this quote path. Missing
	// chapter/character/storyboard context remains an explicit protected-context error.
	runtime.ServerValues = map[string]string{"项目名称": inputs.ProjectName, "项目画风": inputs.StyleType}
	values, err := runtimeValues(TemplateRequest{Operation: definition.Operation, Outline: template.Outline}, runtime)
	if err != nil {
		return Preparation{}, err
	}
	compiled, err := domain.CompileDefinition(definition, template.Customization, values)
	if err != nil || !validQuotePrompt(compiled.Content) {
		return Preparation{}, ErrTemplateInvalid
	}
	prepared.FinalPrompt, prepared.Policy = compiled.Content, "compiled"
	prepared.TemplateID, prepared.TemplateVersion = definition.TemplateID, definition.TemplateVersion
	prepared.CustomizationID, prepared.CustomizationRevision = compiled.CustomizationID, compiled.CustomizationRevision
	prepared.ContentSHA256 = contentHash(prepared.FinalPrompt)
	return prepared, nil
}
