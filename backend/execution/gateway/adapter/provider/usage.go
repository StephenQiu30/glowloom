package provider

import (
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// Usage validates original protocol buckets without conflating missing and zero.
func Usage(protocol string, raw []byte) (domain.Usage, error) {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return domain.Usage{}, domain.ErrUnknown
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(top["usage"], &fields) != nil {
		return domain.Usage{}, domain.ErrUnknown
	}
	return usageFields(protocol, fields)
}
func count(fields map[string]json.RawMessage, key string, required bool) (int64, error) {
	raw, ok := fields[key]
	if !ok {
		if required {
			return 0, domain.ErrUnknown
		}
		return 0, nil
	}
	var n int64
	if string(raw) == "null" || json.Unmarshal(raw, &n) != nil || n < 0 || n > 1<<40 {
		return 0, domain.ErrUnknown
	}
	return n, nil
}
func usageFields(protocol string, f map[string]json.RawMessage) (domain.Usage, error) {
	var u domain.Usage
	var err error
	in, out := "prompt_tokens", "completion_tokens"
	if protocol != "chat" {
		in, out = "input_tokens", "output_tokens"
	}
	u.Input, err = count(f, in, true)
	if err != nil {
		return u, err
	}
	u.Output, err = count(f, out, true)
	if err != nil {
		return u, err
	}
	if protocol == "messages" {
		u.CacheRead, err = count(f, "cache_read_input_tokens", false)
		if err != nil {
			return u, err
		}
		u.CacheWrite, err = count(f, "cache_creation_input_tokens", false)
		if err != nil {
			return u, err
		}
		u.Input += u.CacheRead + u.CacheWrite
	} else {
		name := "prompt_tokens_details"
		if protocol == "responses" {
			name = "input_tokens_details"
		}
		if raw, ok := f[name]; ok {
			var details map[string]json.RawMessage
			if json.Unmarshal(raw, &details) != nil {
				return u, domain.ErrUnknown
			}
			u.CacheRead, err = count(details, "cached_tokens", false)
			if err != nil {
				return u, err
			}
		}
		if _, ok := f["prompt_cache_hit_tokens"]; ok {
			hit, err := count(f, "prompt_cache_hit_tokens", false)
			if err != nil || u.CacheRead != 0 && hit != u.CacheRead {
				return u, domain.ErrUnknown
			}
			u.CacheRead = hit
		}
	}
	if u.Validate() != nil {
		return u, domain.ErrUnknown
	}
	return u, nil
}

type streamState struct {
	expected        string
	modelKnown      bool
	protocol        string
	usage           domain.Usage
	complete, known bool
	input           map[string]json.RawMessage
}

func (s *streamState) consume(e Event) error {
	if e.Data == "[DONE]" {
		if s.protocol != "chat" {
			return domain.ErrUnknown
		}
		s.complete = true
		return nil
	}
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(e.Data), &top) != nil {
		return domain.ErrUnknown
	}
	if raw, ok := top["model"]; ok {
		var model string
		if json.Unmarshal(raw, &model) != nil || model != s.expected {
			return domain.ErrUnknown
		}
		s.modelKnown = true
	}
	var kind string
	_ = json.Unmarshal(top["type"], &kind)
	if kind == "error" || top["error"] != nil {
		return domain.ErrUnknown
	}
	switch s.protocol {
	case "chat":
		if raw, ok := top["usage"]; ok && string(raw) != "null" {
			var f map[string]json.RawMessage
			if json.Unmarshal(raw, &f) != nil {
				return domain.ErrUnknown
			}
			u, err := usageFields("chat", f)
			if err != nil {
				return err
			}
			s.usage = u
			s.known = true
		}
	case "responses":
		if kind == "response.completed" || kind == "response.incomplete" {
			var frame map[string]json.RawMessage
			if json.Unmarshal(top["response"], &frame) != nil {
				return domain.ErrUnknown
			}
			var model string
			if json.Unmarshal(frame["model"], &model) != nil || model != s.expected {
				return domain.ErrUnknown
			}
			s.modelKnown = true
			u, err := Usage("responses", top["response"])
			if err != nil {
				return err
			}
			s.usage = u
			s.complete = true
			s.known = true
		}
	case "messages":
		if kind == "message_start" {
			var message map[string]json.RawMessage
			if json.Unmarshal(top["message"], &message) != nil {
				return domain.ErrUnknown
			}
			var model string
			if json.Unmarshal(message["model"], &model) != nil || model != s.expected {
				return domain.ErrUnknown
			}
			s.modelKnown = true
			if json.Unmarshal(message["usage"], &s.input) != nil {
				return domain.ErrUnknown
			}
		}
		if kind == "message_delta" {
			var delta map[string]json.RawMessage
			if json.Unmarshal(top["usage"], &delta) != nil || s.input == nil {
				return domain.ErrUnknown
			}
			for k, v := range delta {
				s.input[k] = v
			}
			u, err := usageFields("messages", s.input)
			if err != nil {
				return err
			}
			s.usage = u
			s.known = true
		}
		if kind == "message_stop" {
			s.complete = true
		}
	}
	return nil
}
