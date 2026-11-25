package aigateway

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// NormalizeGenerationParams applies the frozen product's supported defaults to creator parameters.
func NormalizeGenerationParams(model domain.Model, mode string, params json.RawMessage) (json.RawMessage, error) {
	values := map[string]json.RawMessage{}
	if len(params) > 0 && json.Unmarshal(params, &values) != nil {
		return nil, domain.ErrInvalid
	}
	var raw json.RawMessage
	if model.Protocol == "media" {
		var scalars map[string]any
		encoded, _ := json.Marshal(values)
		if json.Unmarshal(encoded, &scalars) != nil {
			return nil, domain.ErrInvalid
		}
		raw, _ = json.Marshal(domain.MediaInput{Model: model.Name, Capability: model.Capability, Mode: mode, Prompt: "parameter verification", Params: scalars, OutputCount: 1})
	} else {
		if mode != "text_to_text" {
			return nil, domain.ErrInvalid
		}
		tokens := min(int64(8192), model.MaxOutput)
		if value, ok := values["max_tokens"]; ok {
			if json.Unmarshal(value, &tokens) != nil {
				return nil, domain.ErrInvalid
			}
			delete(values, "max_tokens")
		}
		var err error
		raw, err = (agentInput{ModelKey: model.Name, MaxTokens: int(tokens), Messages: []json.RawMessage{json.RawMessage(`{"role":"user","content":"parameter verification"}`)}, Params: values}).native(model.Protocol)
		if err != nil {
			return nil, err
		}
	}
	call, err := application.Prepare(model, uuid.New(), uuid.New(), uuid.Nil, uuid.Nil, "params", "creation", raw)
	if err != nil {
		return nil, err
	}
	return generationParams(call), nil
}

func generationParams(call domain.Call) json.RawMessage {
	if call.Model.Protocol == "media" {
		var in domain.MediaInput
		_ = json.Unmarshal(call.Raw, &in)
		return canonical(in.Params)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(call.Raw, &fields)
	out := map[string]json.RawMessage{}
	for _, key := range []string{"temperature", "top_p"} {
		if v, ok := fields[key]; ok {
			out[key] = v
		}
	}
	key := "max_tokens"
	if call.Model.Protocol == "responses" {
		key = "max_output_tokens"
	}
	out["max_tokens"] = fields[key]
	return canonical(out)
}
