package provider

import (
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

type toolFragment struct{ ID, Name, Arguments string }
type blockFragment struct {
	Kind, ID, Name, Text, Arguments string
	Closed                          bool
}
type streamEvidence struct {
	protocol, model, content, refusal, finish string
	tools                                     map[int]*toolFragment
	blocks                                    map[int]*blockFragment
	started, ended                            bool
	body                                      json.RawMessage
}

func newEvidence(protocol, model string) *streamEvidence {
	return &streamEvidence{protocol: protocol, model: model, tools: map[int]*toolFragment{}, blocks: map[int]*blockFragment{}}
}
func (s *streamEvidence) consume(e Event) error {
	if s.ended {
		return domain.ErrUnknown
	}
	if e.Data == "[DONE]" {
		if s.protocol != "chat" || s.finish == "" {
			return domain.ErrUnknown
		}
		s.ended = true
		return nil
	}
	var f map[string]json.RawMessage
	if json.Unmarshal([]byte(e.Data), &f) != nil {
		return domain.ErrUnknown
	}
	var kind string
	_ = json.Unmarshal(f["type"], &kind)
	if s.protocol != "chat" && e.Event != "" && e.Event != kind {
		return domain.ErrUnknown
	}
	switch s.protocol {
	case "chat":
		var choices []struct {
			Index  int     `json:"index"`
			Finish *string `json:"finish_reason"`
			Delta  struct {
				Role    string  `json:"role"`
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
				Tools   []struct {
					Index    int `json:"index"`
					ID, Type string
					Function struct{ Name, Arguments string }
				} `json:"tool_calls"`
			} `json:"delta"`
		}
		if json.Unmarshal(f["choices"], &choices) != nil || len(choices) > 1 {
			return domain.ErrUnknown
		}
		for _, c := range choices {
			if c.Index != 0 || s.finish != "" {
				return domain.ErrUnknown
			}
			s.started = true
			if c.Delta.Role != "" && c.Delta.Role != "assistant" {
				return domain.ErrUnknown
			}
			if c.Delta.Content != nil {
				s.content += *c.Delta.Content
			}
			if c.Delta.Refusal != nil {
				s.refusal += *c.Delta.Refusal
			}
			for _, d := range c.Delta.Tools {
				if d.Index < 0 || d.Index > 127 {
					return domain.ErrUnknown
				}
				t := s.tools[d.Index]
				if t == nil {
					t = &toolFragment{}
					s.tools[d.Index] = t
				}
				if d.Type != "" && d.Type != "function" {
					return domain.ErrUnknown
				}
				t.ID += d.ID
				t.Name += d.Function.Name
				t.Arguments += d.Function.Arguments
			}
			if c.Finish != nil {
				s.finish = *c.Finish
				if s.finish != "stop" && s.finish != "length" && s.finish != "tool_calls" {
					return domain.ErrUnknown
				}
			}
		}
		if string(f["usage"]) != "null" && len(f["usage"]) > 0 && s.finish == "" {
			return domain.ErrUnknown
		}
	case "responses":
		if kind == "response.completed" || kind == "response.incomplete" {
			if verifyResponse("responses", f["response"], s.model) != nil {
				return domain.ErrUnknown
			}
			s.body = f["response"]
			s.started = true
			s.ended = true
		}
	case "messages":
		switch kind {
		case "ping":
			return nil
		case "message_start":
			if s.started {
				return domain.ErrUnknown
			}
			s.started = true
		case "content_block_start":
			if !s.started || s.finish != "" {
				return domain.ErrUnknown
			}
			var i int
			if json.Unmarshal(f["index"], &i) != nil || i != len(s.blocks) {
				return domain.ErrUnknown
			}
			var b struct {
				Type, ID, Name, Text string
				Input                map[string]any
			}
			if json.Unmarshal(f["content_block"], &b) != nil || b.Type != "text" && b.Type != "tool_use" {
				return domain.ErrUnknown
			}
			s.blocks[i] = &blockFragment{Kind: b.Type, ID: b.ID, Name: b.Name, Text: b.Text}
			if len(b.Input) > 0 {
				raw, _ := json.Marshal(b.Input)
				s.blocks[i].Arguments = string(raw)
			}
		case "content_block_delta":
			var i int
			if json.Unmarshal(f["index"], &i) != nil {
				return domain.ErrUnknown
			}
			b := s.blocks[i]
			if b == nil || b.Closed {
				return domain.ErrUnknown
			}
			var d struct {
				Type, Text string
				Partial    string `json:"partial_json"`
			}
			if json.Unmarshal(f["delta"], &d) != nil {
				return domain.ErrUnknown
			}
			switch {
			case d.Type == "text_delta" && b.Kind == "text":
				b.Text += d.Text
			case d.Type == "input_json_delta" && b.Kind == "tool_use":
				b.Arguments += d.Partial
			default:
				return domain.ErrUnknown
			}
		case "content_block_stop":
			var i int
			if json.Unmarshal(f["index"], &i) != nil || s.blocks[i] == nil || s.blocks[i].Closed {
				return domain.ErrUnknown
			}
			s.blocks[i].Closed = true
		case "message_delta":
			var d struct {
				Stop string `json:"stop_reason"`
			}
			if json.Unmarshal(f["delta"], &d) != nil || d.Stop == "" || s.finish != "" {
				return domain.ErrUnknown
			}
			for _, b := range s.blocks {
				if !b.Closed {
					return domain.ErrUnknown
				}
			}
			s.finish = d.Stop
		case "message_stop":
			if !s.started || s.finish == "" {
				return domain.ErrUnknown
			}
			s.ended = true
		default:
			return domain.ErrUnknown
		}
	}
	return nil
}
func (s *streamEvidence) response() (json.RawMessage, error) {
	if !s.started || !s.ended {
		return nil, domain.ErrUnknown
	}
	if s.protocol == "responses" {
		return s.body, nil
	}
	var body any
	if s.protocol == "chat" {
		tools := []any{}
		for i := 0; i < len(s.tools); i++ {
			t := s.tools[i]
			if t == nil {
				return nil, domain.ErrUnknown
			}
			tools = append(tools, map[string]any{"id": t.ID, "type": "function", "function": map[string]any{"name": t.Name, "arguments": t.Arguments}})
		}
		message := map[string]any{"role": "assistant", "content": s.content, "tool_calls": tools}
		if s.refusal != "" {
			message["refusal"] = s.refusal
		}
		body = map[string]any{"model": s.model, "choices": []any{map[string]any{"message": message, "finish_reason": s.finish}}}
	} else {
		blocks := []any{}
		hasTool := false
		for i := 0; i < len(s.blocks); i++ {
			b := s.blocks[i]
			if b == nil || !b.Closed {
				return nil, domain.ErrUnknown
			}
			if b.Kind == "text" {
				blocks = append(blocks, map[string]any{"type": "text", "text": b.Text})
			} else {
				hasTool = true
				args := json.RawMessage(b.Arguments)
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": args})
			}
		}
		if hasTool != (s.finish == "tool_use") {
			return nil, domain.ErrUnknown
		}
		body = map[string]any{"model": s.model, "type": "message", "content": blocks, "stop_reason": s.finish}
	}
	raw, err := json.Marshal(body)
	if err != nil || verifyResponse(s.protocol, raw, s.model) != nil {
		return nil, domain.ErrUnknown
	}
	return raw, nil
}

func verifyToolNames(protocol string, request, body json.RawMessage) error {
	var req map[string]json.RawMessage
	var defs []map[string]json.RawMessage
	if json.Unmarshal(request, &req) != nil {
		return domain.ErrUnknown
	}
	_ = json.Unmarshal(req["tools"], &defs)
	allowed := map[string]bool{}
	for _, d := range defs {
		var name string
		if protocol == "chat" {
			var f struct{ Name string }
			_ = json.Unmarshal(d["function"], &f)
			name = f.Name
		} else {
			_ = json.Unmarshal(d["name"], &name)
		}
		allowed[name] = true
	}
	var visit func(any) error
	visit = func(v any) error {
		switch x := v.(type) {
		case []any:
			for _, a := range x {
				if err := visit(a); err != nil {
					return err
				}
			}
		case map[string]any:
			kind, _ := x["type"].(string)
			if kind == "tool_use" || kind == "function_call" {
				name, _ := x["name"].(string)
				if !allowed[name] {
					return domain.ErrUnknown
				}
			}
			if calls, ok := x["tool_calls"].([]any); ok {
				ids := map[string]bool{}
				for _, a := range calls {
					t, ok := a.(map[string]any)
					if !ok {
						return domain.ErrUnknown
					}
					id, _ := t["id"].(string)
					if ids[id] {
						return domain.ErrUnknown
					}
					ids[id] = true
					f, ok := t["function"].(map[string]any)
					if !ok {
						return domain.ErrUnknown
					}
					name, _ := f["name"].(string)
					if !allowed[name] {
						return domain.ErrUnknown
					}
				}
			}
			// Traverse output containers only; user tool arguments may contain arbitrary keys.
			for _, k := range []string{"choices", "message", "output", "content"} {
				if a, ok := x[k]; ok {
					if err := visit(a); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	var tree any
	if json.Unmarshal(body, &tree) != nil {
		return domain.ErrUnknown
	}
	return visit(tree)
}
