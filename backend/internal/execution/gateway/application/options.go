package application

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func validateOptions(protocol string, f map[string]json.RawMessage, names map[string]bool) error {
	for key, bounds := range map[string][2]float64{"temperature": {0, 2}, "top_p": {0, 1}, "frequency_penalty": {-2, 2}, "presence_penalty": {-2, 2}} {
		if v, ok := f[key]; ok {
			var n float64
			if json.Unmarshal(v, &n) != nil || math.IsNaN(n) || n < bounds[0] || n > bounds[1] {
				return domain.ErrInvalid
			}
			if protocol == "messages" && key == "temperature" && n > 1 {
				return domain.ErrInvalid
			}
		}
	}
	if v, ok := f["parallel_tool_calls"]; ok {
		var value bool
		if json.Unmarshal(v, &value) != nil {
			return domain.ErrInvalid
		}
	}
	if v, ok := f["seed"]; ok {
		var value int64
		if json.Unmarshal(v, &value) != nil {
			return domain.ErrInvalid
		}
	}
	if v, ok := f["stream_options"]; ok {
		var opts map[string]json.RawMessage
		if json.Unmarshal(v, &opts) != nil || len(opts) != 1 || string(opts["include_usage"]) != "true" {
			return domain.ErrInvalid
		}
	}
	for _, key := range []string{"stop", "stop_sequences"} {
		if v, ok := f[key]; ok {
			var list []string
			var single string
			if key == "stop" && json.Unmarshal(v, &single) == nil {
				list = []string{single}
			} else if json.Unmarshal(v, &list) != nil {
				return domain.ErrInvalid
			}
			if len(list) > 4 {
				return domain.ErrInvalid
			}
			for _, s := range list {
				if s == "" || len(s) > 2048 {
					return domain.ErrInvalid
				}
			}
		}
	}
	if v, ok := f["instructions"]; ok {
		var text string
		if json.Unmarshal(v, &text) != nil {
			return domain.ErrInvalid
		}
	}
	if v, ok := f["system"]; ok && !textContent(v, protocol) {
		return domain.ErrInvalid
	}
	if v, ok := f["tool_choice"]; ok {
		var value string
		if json.Unmarshal(v, &value) == nil {
			if protocol == "messages" || !slices.Contains([]string{"auto", "none", "required"}, value) {
				return domain.ErrInvalid
			}
			return nil
		}
		var choice map[string]json.RawMessage
		if json.Unmarshal(v, &choice) != nil {
			return domain.ErrInvalid
		}
		var kind, name string
		_ = json.Unmarshal(choice["type"], &kind)
		if protocol == "messages" {
			if !slices.Contains([]string{"auto", "any", "tool", "none"}, kind) {
				return domain.ErrInvalid
			}
			if kind != "tool" {
				if _, exists := choice["name"]; exists {
					return domain.ErrInvalid
				}
			}
			if kind == "tool" {
				_ = json.Unmarshal(choice["name"], &name)
				if !names[name] {
					return domain.ErrInvalid
				}
			}
			if raw, ok := choice["disable_parallel_tool_use"]; ok {
				var enabled bool
				if json.Unmarshal(raw, &enabled) != nil {
					return domain.ErrInvalid
				}
			}
			for k := range choice {
				if k != "type" && k != "name" && k != "disable_parallel_tool_use" {
					return domain.ErrInvalid
				}
			}
			return nil
		}
		if kind != "function" {
			return domain.ErrInvalid
		}
		if protocol == "chat" {
			if len(choice) != 2 {
				return domain.ErrInvalid
			}
			var functionFields map[string]json.RawMessage
			if json.Unmarshal(choice["function"], &functionFields) != nil || len(functionFields) != 1 {
				return domain.ErrInvalid
			}
			var function struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(choice["function"], &function) != nil {
				return domain.ErrInvalid
			}
			name = function.Name
		} else {
			if len(choice) != 2 {
				return domain.ErrInvalid
			}
			_ = json.Unmarshal(choice["name"], &name)
		}
		if !names[name] {
			return domain.ErrInvalid
		}
	}
	return nil
}
