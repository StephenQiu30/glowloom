package aigateway

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// Estimate runs the same native preparation and exact sale arithmetic without writes.
func (c Creation) Estimate(ctx context.Context, a identity.Principal, p uuid.UUID, d promptdomain.Draft) (operation.GenerationEstimate, error) {
	var out operation.GenerationEstimate
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		if d.Validate() != nil || d.ModelVersion == nil || d.Mode == nil {
			return domain.ErrInvalid
		}
		var row struct{ Document []byte }
		q := tx.Raw("SELECT document FROM ai.model WHERE id=? AND enabled", *d.ModelVersion).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		var m domain.Model
		if q.RowsAffected != 1 || json.Unmarshal(row.Document, &m) != nil {
			return domain.ErrUnavailable
		}
		compiled, err := d.Compile()
		if err != nil {
			return domain.ErrInvalid
		}
		if m.Protocol == "media" {
			refs := []domain.Reference{}
			for _, ref := range compiled.Media {
				id, err := uuid.Parse(ref.SourceID)
				if err != nil {
					return domain.ErrInvalid
				}
				input := domain.Reference{ID: id, Role: ref.Role, Version: ref.SourceVersion}
				if ref.BindingID != "" {
					input.BindingID, err = uuid.Parse(ref.BindingID)
					if err != nil {
						return domain.ErrInvalid
					}
				}
				refs = append(refs, input)
			}
			refs, err = c.Runtime.freezeReferences(ctx, tx, a, p, m, refs)
			if err != nil {
				return err
			}
			params := map[string]any{}
			for name, value := range d.Params {
				if name == "output_count" {
					if value != float64(1) {
						return domain.ErrInvalid
					}
				} else {
					params[name] = value
				}
			}
			raw := canonical(domain.MediaInput{Model: m.Name, Capability: m.Capability, Mode: *d.Mode, Prompt: compiled.Prompt, Params: params, References: refs, OutputCount: 1})
			call, err := application.Prepare(m, a.ID, a.ScopeID, p, uuid.Nil, "estimate", "creation", raw)
			if err != nil {
				return err
			}
			if m.VideoMetering != nil {
				credits, err := call.VideoEstimatedCredits()
				if err != nil {
					return err
				}
				out = operation.GenerationEstimate{Credits: strconv.FormatInt(credits, 10), Kind: "estimate", Basis: "video_tokens", Quantity: strconv.FormatInt(call.EstimatedVideoTokens, 10)}
				return nil
			}
			out = operation.GenerationEstimate{Credits: strconv.FormatInt(call.Limit, 10), Kind: "estimate", Basis: "per_request", Quantity: "1"}
			return nil
		}
		if len(compiled.Media) > 0 || *d.Mode != "text_to_text" {
			return domain.ErrInvalid
		}
		maxTokens := min(int64(8192), m.MaxOutput)
		params := map[string]json.RawMessage{}
		for k, v := range d.Params {
			switch k {
			case "max_tokens":
				if json.Unmarshal(canonical(v), &maxTokens) != nil {
					return domain.ErrInvalid
				}
			case "output_count":
				if v != float64(1) {
					return domain.ErrInvalid
				}
			default:
				params[k] = canonical(v)
			}
		}
		raw, err := (agentInput{ModelKey: m.Name, MaxTokens: int(maxTokens), Messages: []json.RawMessage{canonical(map[string]string{"role": "user", "content": compiled.Prompt})}, Params: params}).native(m.Protocol)
		if err != nil {
			return err
		}
		call, err := application.Prepare(m, a.ID, a.ScopeID, p, uuid.Nil, "estimate", "creation", raw)
		if err != nil {
			return err
		}
		out = operation.GenerationEstimate{Credits: strconv.FormatInt(call.Limit, 10), Kind: "maximum", Basis: "request"}
		return nil
	})
	return out, err
}
