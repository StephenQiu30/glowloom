package aigateway

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
)

func creationSource(in operationhttp.QuoteItemRequest) []byte {
	if in.InputSource != nil {
		return canonical(in.InputSource)
	}
	return canonical(in.Source)
}
func nativePrompt(m domain.Model, raw json.RawMessage) string {
	if m.Protocol == "media" {
		var v domain.MediaInput
		_ = json.Unmarshal(raw, &v)
		return v.Prompt
	}
	var v struct {
		Input    string `json:"input"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &v)
	if len(v.Messages) > 0 {
		return v.Messages[0].Content
	}
	return v.Input
}
func (c Creation) quoteInput(ctx context.Context, tx *gorm.DB, a identity.Principal, p uuid.UUID, in operationhttp.QuoteItemRequest) (domain.Model, json.RawMessage, *promptdomain.Draft, error) {
	var model domain.Model
	var draft *promptdomain.Draft
	name, mode, text, params := in.ModelKey, in.Mode, in.Prompt, in.Params
	refs := []domain.Reference{}
	for _, item := range in.MediaInputs {
		ref := domain.Reference{ID: item.MediaAssetID, Role: item.Role}
		if item.SubjectBindingID != nil {
			ref.BindingID = *item.SubjectBindingID
		}
		refs = append(refs, ref)
	}
	if in.OutputCount > 1 || in.OutputCount < 0 {
		return model, nil, nil, domain.ErrInvalid
	}
	if in.Source != nil {
		if !in.Source.Valid() || in.Source.RowID != nil {
			return model, nil, nil, domain.ErrInvalid
		}
		var n int64
		if err := tx.Raw("SELECT count(*) FROM canvas.document d JOIN canvas.node n ON n.document_id=d.id WHERE d.id=? AND d.project_id=? AND d.revision=? AND n.id=? AND NOT d.is_delete AND NOT n.is_delete", in.Source.CanvasID, p, in.Source.Revision, in.Source.NodeID).Scan(&n).Error; err != nil {
			return model, nil, nil, err
		}
		if n != 1 {
			return model, nil, nil, domain.ErrConflict
		}
	}
	if in.InputSource != nil {
		d, err := operationwire.ProvidePromptEditor(tx).ReadInput(ctx, a, p, *in.InputSource, false)
		if err != nil {
			return model, nil, nil, err
		}
		draft = &d
		if d.ModelVersion == nil || d.Mode == nil {
			return model, nil, nil, domain.ErrInvalid
		}
		var row struct{ Document []byte }
		q := tx.Raw("SELECT document FROM ai.model WHERE id=? AND enabled", *d.ModelVersion).Scan(&row)
		if q.Error != nil {
			return model, nil, nil, q.Error
		}
		if q.RowsAffected != 1 || json.Unmarshal(row.Document, &model) != nil {
			return model, nil, nil, domain.ErrUnavailable
		}
		compiled, err := d.Compile()
		if err != nil {
			return model, nil, nil, err
		}
		name = model.Name
		mode = *d.Mode
		text = compiled.Prompt
		for _, m := range compiled.Media {
			id, err := uuid.Parse(m.SourceID)
			if err != nil {
				return model, nil, nil, domain.ErrInvalid
			}
			ref := domain.Reference{ID: id, Role: m.Role, Version: m.SourceVersion}
			if m.BindingID != "" {
				ref.BindingID, err = uuid.Parse(m.BindingID)
				if err != nil {
					return model, nil, nil, domain.ErrInvalid
				}
			}
			refs = append(refs, ref)
		}
		values := map[string]any{}
		for k, v := range d.Params {
			if k == "output_count" {
				if v != float64(1) {
					return model, nil, nil, domain.ErrInvalid
				}
			} else {
				values[k] = v
			}
		}
		params = canonical(values)
	} else {
		if name == "" {
			var defaults struct{ DefaultModels []byte }
			if err := tx.Raw("SELECT default_models FROM workspace.project WHERE id=?", p).Scan(&defaults).Error; err != nil {
				return model, nil, nil, err
			}
			var models map[string]string
			if json.Unmarshal(defaults.DefaultModels, &models) != nil {
				return model, nil, nil, domain.ErrInvalid
			}
			name = models[in.Capability]
		}
		var rows []struct{ Document []byte }
		if err := tx.Raw("SELECT document FROM ai.model WHERE name=? AND enabled ORDER BY id", name).Scan(&rows).Error; err != nil {
			return model, nil, nil, err
		}
		if len(rows) != 1 || json.Unmarshal(rows[0].Document, &model) != nil {
			return model, nil, nil, domain.ErrUnavailable
		}
	}
	capability := "text.generate"
	if model.Protocol == "media" {
		capability = model.Capability
	}
	if in.Capability != "" && in.Capability != capability && (capability != "text.generate" || in.Capability != "text.structured") {
		return model, nil, draft, domain.ErrInvalid
	}
	if in.PromptTemplate != nil {
		var project struct{ AspectRatio, Resolution string }
		if err := tx.Raw("SELECT aspect_ratio,resolution FROM workspace.project WHERE id=?", p).Scan(&project).Error; err != nil {
			return model, nil, draft, err
		}
		prepared, err := operationwire.ProvidePromptCompiler(tx).Prepare(ctx, a, *in.PromptTemplate, prompt.RuntimeContext{ProjectID: p, Capability: in.Capability, Mode: mode, UserPrompt: text, ServerValues: map[string]string{"aspect_ratio": project.AspectRatio, "resolution": project.Resolution}})
		if err != nil {
			return model, nil, draft, err
		}
		text = prepared.FinalPrompt
	}
	if model.Protocol == "media" {
		values := map[string]any{}
		if len(params) > 0 && json.Unmarshal(params, &values) != nil {
			return model, nil, draft, domain.ErrInvalid
		}
		resolved, err := c.Runtime.freezeReferences(ctx, tx, a, p, refs)
		if err != nil {
			return model, nil, draft, err
		}
		return model, canonical(domain.MediaInput{Model: name, Capability: capability, Mode: mode, Prompt: text, Params: values, References: resolved, OutputCount: 1}), draft, nil
	}
	if len(refs) > 0 || mode != "text_to_text" || text == "" {
		return model, nil, draft, domain.ErrInvalid
	}
	values := map[string]json.RawMessage{}
	if len(params) > 0 && json.Unmarshal(params, &values) != nil {
		return model, nil, draft, domain.ErrInvalid
	}
	maxTokens := min(int64(8192), model.MaxOutput)
	if v, ok := values["max_tokens"]; ok {
		if json.Unmarshal(v, &maxTokens) != nil {
			return model, nil, draft, domain.ErrInvalid
		}
		delete(values, "max_tokens")
	}
	raw, err := (agentInput{ModelKey: name, MaxTokens: int(maxTokens), Messages: []json.RawMessage{canonical(map[string]any{"role": "user", "content": text})}, Params: values}).native(model.Protocol)
	return model, raw, draft, err
}
