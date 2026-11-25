package provider

import (
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func verifyResponse(protocol string, raw []byte, expected string) error {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top["error"] != nil {
		return domain.ErrUnknown
	}
	var model string
	if json.Unmarshal(top["model"], &model) != nil || model != expected {
		return domain.ErrUnknown
	}
	switch protocol {
	case "chat":
		var choices []struct {
			Finish  string `json:"finish_reason"`
			Message struct {
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
				Tools   []struct {
					ID       string                           `json:"id"`
					Type     string                           `json:"type"`
					Function struct{ Name, Arguments string } `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		}
		if json.Unmarshal(top["choices"], &choices) != nil || len(choices) != 1 {
			return domain.ErrUnknown
		}
		choice := choices[0]
		if choice.Finish != "stop" && choice.Finish != "length" && choice.Finish != "tool_calls" {
			return domain.ErrUnknown
		}
		if (choice.Finish == "tool_calls") != (len(choice.Message.Tools) > 0) {
			return domain.ErrUnknown
		}
		if choice.Message.Content == nil && choice.Message.Refusal == nil && len(choice.Message.Tools) == 0 {
			return domain.ErrUnknown
		}
		for _, tool := range choice.Message.Tools {
			var args map[string]any
			if tool.ID == "" || tool.Type != "function" || tool.Function.Name == "" || json.Unmarshal([]byte(tool.Function.Arguments), &args) != nil || args == nil {
				return domain.ErrUnknown
			}
		}
	case "responses":
		var state string
		_ = json.Unmarshal(top["status"], &state)
		if state != "completed" && state != "incomplete" {
			return domain.ErrUnknown
		}
		var output []map[string]json.RawMessage
		if json.Unmarshal(top["output"], &output) != nil || len(output) == 0 {
			return domain.ErrUnknown
		}
		for _, item := range output {
			var kind string
			_ = json.Unmarshal(item["type"], &kind)
			if kind != "message" && kind != "function_call" && kind != "reasoning" {
				return domain.ErrUnknown
			}
			if kind == "function_call" {
				var name, id, args string
				_ = json.Unmarshal(item["name"], &name)
				_ = json.Unmarshal(item["call_id"], &id)
				_ = json.Unmarshal(item["arguments"], &args)
				var object map[string]any
				if name == "" || id == "" || json.Unmarshal([]byte(args), &object) != nil || object == nil {
					return domain.ErrUnknown
				}
			}
		}
	case "messages":
		var kind, stop string
		_ = json.Unmarshal(top["type"], &kind)
		_ = json.Unmarshal(top["stop_reason"], &stop)
		if kind != "message" || stop != "end_turn" && stop != "max_tokens" && stop != "tool_use" && stop != "stop_sequence" {
			return domain.ErrUnknown
		}
		var blocks []map[string]json.RawMessage
		if json.Unmarshal(top["content"], &blocks) != nil || len(blocks) == 0 {
			return domain.ErrUnknown
		}
		for _, b := range blocks {
			var kind string
			_ = json.Unmarshal(b["type"], &kind)
			if kind != "text" && kind != "tool_use" {
				return domain.ErrUnknown
			}
			if kind == "tool_use" {
				var id, name string
				_ = json.Unmarshal(b["id"], &id)
				_ = json.Unmarshal(b["name"], &name)
				var args map[string]any
				if id == "" || name == "" || json.Unmarshal(b["input"], &args) != nil || args == nil {
					return domain.ErrUnknown
				}
			}
		}
	default:
		return domain.ErrUnknown
	}
	return nil
}
