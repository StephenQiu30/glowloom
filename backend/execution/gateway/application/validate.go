package application

import (
	"encoding/json"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// validateNative validates the admitted subset, rather than relying on an upstream 400.
func validateNative(protocol string, fields map[string]json.RawMessage, toolsEnabled bool) error {
	names := map[string]bool{}
	if raw, ok := fields["tools"]; ok {
		if !toolsEnabled {
			return domain.ErrInvalid
		}
		var list []map[string]json.RawMessage
		if json.Unmarshal(raw, &list) != nil || len(list) > 128 {
			return domain.ErrInvalid
		}
		for _, tool := range list {
			var name string
			var schema json.RawMessage
			switch protocol {
			case "chat":
				var f struct {
					Name       string          `json:"name"`
					Parameters json.RawMessage `json:"parameters"`
				}
				if json.Unmarshal(tool["function"], &f) != nil {
					return domain.ErrInvalid
				}
				name = f.Name
				schema = f.Parameters
			case "responses":
				_ = json.Unmarshal(tool["name"], &name)
				schema = tool["parameters"]
			default:
				_ = json.Unmarshal(tool["name"], &name)
				schema = tool["input_schema"]
			}
			var shape struct {
				Type string `json:"type"`
			}
			if name == "" || len(name) > 128 || names[name] || json.Unmarshal(schema, &shape) != nil || shape.Type != "object" {
				return domain.ErrInvalid
			}
			names[name] = true
		}
	}
	if _, present := fields["tool_choice"]; present && len(names) == 0 {
		return domain.ErrInvalid
	}
	if _, present := fields["parallel_tool_calls"]; present && !toolsEnabled {
		return domain.ErrInvalid
	}
	if err := validateOptions(protocol, fields, names); err != nil {
		return err
	}
	if protocol == "responses" {
		var text string
		if json.Unmarshal(fields["input"], &text) == nil {
			if strings.TrimSpace(text) == "" {
				return domain.ErrInvalid
			}
			return nil
		}
		var items []map[string]json.RawMessage
		if json.Unmarshal(fields["input"], &items) != nil || len(items) == 0 {
			return domain.ErrInvalid
		}
		pending := map[string]bool{}
		for _, item := range items {
			var kind, role string
			_ = json.Unmarshal(item["type"], &kind)
			_ = json.Unmarshal(item["role"], &role)
			if kind == "function_call_output" {
				var id, output string
				if json.Unmarshal(item["call_id"], &id) != nil || id == "" || !pending[id] || json.Unmarshal(item["output"], &output) != nil {
					return domain.ErrInvalid
				}
				delete(pending, id)
				continue
			}
			if kind == "function_call" {
				var id, name, args string
				_ = json.Unmarshal(item["call_id"], &id)
				_ = json.Unmarshal(item["name"], &name)
				_ = json.Unmarshal(item["arguments"], &args)
				var object map[string]any
				if id == "" || pending[id] || !names[name] || json.Unmarshal([]byte(args), &object) != nil || object == nil {
					return domain.ErrInvalid
				}
				pending[id] = true
				continue
			}
			if kind != "" && kind != "message" || role != "user" && role != "assistant" && role != "system" && role != "developer" || !textContent(item["content"], protocol) {
				return domain.ErrInvalid
			}
		}
		if len(pending) > 0 {
			return domain.ErrInvalid
		}
		return nil
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(fields["messages"], &messages) != nil || len(messages) == 0 {
		return domain.ErrInvalid
	}
	pending := map[string]bool{}
	for _, m := range messages {
		var role string
		if json.Unmarshal(m["role"], &role) != nil {
			return domain.ErrInvalid
		}
		if protocol == "chat" {
			if role != "user" && role != "assistant" && role != "system" && role != "developer" && role != "tool" {
				return domain.ErrInvalid
			}
			var calls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}
			if raw, ok := m["tool_calls"]; ok {
				if role != "assistant" || json.Unmarshal(raw, &calls) != nil {
					return domain.ErrInvalid
				}
				for _, call := range calls {
					var object map[string]any
					if call.ID == "" || call.Type != "function" || !names[call.Function.Name] || pending[call.ID] || json.Unmarshal([]byte(call.Function.Arguments), &object) != nil || object == nil {
						return domain.ErrInvalid
					}
					pending[call.ID] = true
				}
			}
			if len(calls) == 0 && !textContent(m["content"], protocol) {
				return domain.ErrInvalid
			}
			if role == "tool" {
				var id string
				if json.Unmarshal(m["tool_call_id"], &id) != nil || !pending[id] {
					return domain.ErrInvalid
				}
				delete(pending, id)
			}
		} else {
			if role != "user" && role != "assistant" {
				return domain.ErrInvalid
			}
			var text string
			if json.Unmarshal(m["content"], &text) == nil {
				if !textContent(m["content"], protocol) {
					return domain.ErrInvalid
				}
				continue
			}
			var blocks []map[string]json.RawMessage
			if json.Unmarshal(m["content"], &blocks) != nil || len(blocks) == 0 {
				return domain.ErrInvalid
			}
			for _, block := range blocks {
				var kind string
				_ = json.Unmarshal(block["type"], &kind)
				switch kind {
				case "text":
					if !textContent(block["text"], protocol) {
						return domain.ErrInvalid
					}
				case "tool_use":
					var id, name string
					_ = json.Unmarshal(block["id"], &id)
					_ = json.Unmarshal(block["name"], &name)
					var object map[string]any
					if role != "assistant" || id == "" || !names[name] || pending[id] || json.Unmarshal(block["input"], &object) != nil || object == nil {
						return domain.ErrInvalid
					}
					pending[id] = true
				case "tool_result":
					var id string
					_ = json.Unmarshal(block["tool_use_id"], &id)
					if role != "user" || !pending[id] || !textContent(block["content"], protocol) {
						return domain.ErrInvalid
					}
					delete(pending, id)
				default:
					return domain.ErrInvalid
				}
			}
		}
	}
	if len(pending) > 0 {
		return domain.ErrInvalid
	}
	return nil
}
func textContent(raw json.RawMessage, protocol string) bool {
	if string(raw) == "null" {
		return false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return true
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil || len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part.Type != "text" && (protocol != "responses" || (part.Type != "input_text" && part.Type != "output_text")) {
			return false
		}
	}
	return true
}
