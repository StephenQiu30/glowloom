package application

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// Prepare accepts a documented native subset. Unsupported fields fail before funds or I/O.
func Prepare(model domain.Model, actor, scope, project, run uuid.UUID, key, origin string, raw json.RawMessage) (domain.Call, error) {
	c := domain.Call{ID: uuid.New(), Actor: actor, Scope: scope, Project: project, Run: run, Key: key, Origin: origin, Model: model, Deadline: time.Now().Add(180 * time.Second)}
	if model.Validate() != nil || len(raw) > 1<<20 || key == "" || len(key) > 160 {
		return c, domain.ErrInvalid
	}
	if model.Protocol == domain.EmbeddingProtocol {
		return prepareEmbedding(c, raw)
	}
	if model.Protocol == "local" {
		return prepareLocal(c, raw)
	}
	if model.Protocol == "media" {
		return prepareMedia(c, raw)
	}
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	if dec.Decode(&fields) != nil || dec.Decode(new(any)) != io.EOF || len(fields) == 0 {
		return c, domain.ErrInvalid
	}
	allowed := map[string]bool{"model": true, "stream": true, "temperature": true, "top_p": true, "tools": true, "tool_choice": true}
	switch model.Protocol {
	case "chat":
		for _, k := range []string{"messages", "max_tokens", "max_completion_tokens", "parallel_tool_calls", "frequency_penalty", "presence_penalty", "seed", "stop", "stream_options"} {
			allowed[k] = true
		}
	case "responses":
		for _, k := range []string{"input", "instructions", "max_output_tokens", "parallel_tool_calls", "store"} {
			allowed[k] = true
		}
	case "messages":
		for _, k := range []string{"messages", "system", "max_tokens", "stop_sequences"} {
			allowed[k] = true
		}
	}
	for k := range fields {
		if !allowed[k] {
			return c, domain.ErrInvalid
		}
	}
	var selected string
	if json.Unmarshal(fields["model"], &selected) != nil || selected != model.Name {
		return c, domain.ErrInvalid
	}
	if raw, ok := fields["stream"]; ok {
		if json.Unmarshal(raw, &c.Stream) != nil || c.Stream && !model.Stream {
			return c, domain.ErrInvalid
		}
	}
	outputKey := "max_tokens"
	if model.Protocol == "responses" {
		outputKey = "max_output_tokens"
	}
	if _, ok := fields["max_completion_tokens"]; ok {
		if _, both := fields["max_tokens"]; both {
			return c, domain.ErrInvalid
		}
		outputKey = "max_completion_tokens"
	}
	var output int64
	if json.Unmarshal(fields[outputKey], &output) != nil || output < 1 || output > model.MaxOutput {
		return c, domain.ErrInvalid
	}
	if model.Protocol == "responses" {
		if _, ok := fields["input"]; !ok {
			return c, domain.ErrInvalid
		}
		if raw, ok := fields["store"]; ok && string(raw) != "false" {
			return c, domain.ErrInvalid
		}
		fields["store"] = json.RawMessage("false")
	} else {
		var messages []map[string]json.RawMessage
		if json.Unmarshal(fields["messages"], &messages) != nil || len(messages) == 0 {
			return c, domain.ErrInvalid
		}
	}
	if raw, ok := fields["tools"]; ok {
		var tools []map[string]json.RawMessage
		if !model.Tools || json.Unmarshal(raw, &tools) != nil || len(tools) > 128 {
			return c, domain.ErrInvalid
		}
		for _, tool := range tools {
			var kind string
			_ = json.Unmarshal(tool["type"], &kind)
			if model.Protocol == "messages" {
				if kind != "" {
					return c, domain.ErrInvalid
				}
			} else if kind != "function" {
				return c, domain.ErrInvalid
			}
		}
	}
	// No remote images/files or hosted tools in the text contract. Tool input/output
	// payloads remain native JSON and are never executed by the gateway.
	var tree any
	if json.Unmarshal(raw, &tree) != nil || !textOnly(tree) {
		return c, domain.ErrInvalid
	}
	if err := validateNative(model.Protocol, fields, model.Tools); err != nil {
		return c, err
	}
	encodedFields, _ := json.Marshal(fields)
	canonical, err := domain.CanonicalJSON(encodedFields)
	if err != nil {
		return c, err
	}
	if int64(len(canonical))+1024 > model.MaxInput {
		return c, domain.ErrInvalid
	}
	c.Raw = canonical
	hashInput := struct {
		Scope, Actor, Project, Run uuid.UUID
		Origin                     string
		Raw                        json.RawMessage
	}{scope, actor, project, run, origin, canonical}
	encoded, _ := json.Marshal(hashInput)
	c.Hash = domain.Digest(encoded)
	// Reserve using the highest applicable input bucket. Supplier hard ceilings
	// remain enforced by its reviewed model contract, not an SDK default.
	price := model.Price
	for _, v := range []int64{price.CacheRead, price.CacheWrite} {
		if v > price.Input {
			price.Input = v
		}
	}
	amount, err := price.Cost(domain.Usage{Input: int64(len(canonical)) + 1024, Output: output})
	if err != nil {
		return c, err
	}
	c.Limit, err = domain.Credits(amount)
	return c, err
}
func textOnly(v any) bool {
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			if !textOnly(item) {
				return false
			}
		}
	case map[string]any:
		if _, ok := t["cache_control"]; ok {
			return false
		}
		if kind, ok := t["type"].(string); ok {
			switch kind {
			case "image", "image_url", "input_image", "file", "input_file", "computer", "web_search", "file_search":
				return false
			}
		}
		for key, item := range t {
			if key == "parameters" || key == "input_schema" {
				continue
			}
			if key == "input" && t["type"] == "tool_use" {
				continue
			}
			if !textOnly(item) {
				return false
			}
		}
	}
	return true
}
