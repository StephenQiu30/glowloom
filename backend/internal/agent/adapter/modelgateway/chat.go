// Package modelgateway dispatches frozen dialogue requests through Go-owned catalog credentials.
package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

var errInput = errors.New("invalid model transcript")
var errResponse = errors.New("unverified model response")

type function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type wireCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function function `json:"function"`
}
type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []wireCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}
type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
type chatInput struct {
	ModelKey  string                     `json:"model_key"`
	ReleaseID string                     `json:"model_release_id,omitempty"`
	Messages  []message                  `json:"messages"`
	Tools     []tool                     `json:"tools"`
	MaxTokens int                        `json:"max_tokens"`
	Params    map[string]json.RawMessage `json:"params"`
}

// ToolCall contains one model-requested function and its decoded arguments.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Usage contains provider-reported input, output and cache token counts.
type Usage struct {
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  *int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64 `json:"cache_write_tokens,omitempty"`
}

// Result contains verified model output, tool calls and token usage.
type Result struct {
	Output     string     `json:"output"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Usage      Usage      `json:"usage"`
	ModelKey   string     `json:"model_key"`
	StopReason string     `json:"stop_reason"`
}

// ChatClient sends bounded chat requests through the configured HTTP client.
type ChatClient struct {
	client      http.Client
	idleTimeout time.Duration
}

// NewChatClient bounds idle provider progress at 45 seconds. An optional
// shorter bound supports isolated protocol tests; it cannot relax production.
func NewChatClient(client *http.Client, shorterIdle ...time.Duration) *ChatClient {
	var c http.Client
	if client != nil {
		c = *client
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	idle := 45 * time.Second
	if len(shorterIdle) == 1 && shorterIdle[0] > 0 && shorterIdle[0] < idle {
		idle = shorterIdle[0]
	}
	return &ChatClient{client: c, idleTimeout: idle}
}
func parseChat(raw json.RawMessage) (chatInput, error) {
	var in chatInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if len(raw) > agent.MaxModelRequestBytes || dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF || in.ModelKey == "" || in.MaxTokens < 1 || in.MaxTokens > 65536 || len(in.Messages) == 0 || len(in.Tools) > 128 {
		return in, errInput
	}
	names := map[string]bool{}
	for _, t := range in.Tools {
		var schema map[string]any
		if t.Name == "" || len(t.Name) > 128 || names[t.Name] || json.Unmarshal(t.Parameters, &schema) != nil || schema["type"] != "object" {
			return in, errInput
		}
		names[t.Name] = true
	}
	for _, m := range in.Messages {
		switch m.Role {
		case "system", "user", "assistant", "tool":
		default:
			return in, errInput
		}
		if m.Role == "tool" && m.ToolCallID == "" || len(m.ToolCalls) > 0 && m.Role != "assistant" {
			return in, errInput
		}
		if len(m.Content) == 0 && len(m.ToolCalls) == 0 {
			return in, errInput
		}
		if len(m.Content) > 0 && string(m.Content) != "null" {
			var value string
			if json.Unmarshal(m.Content, &value) != nil {
				var parts []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL *struct {
						URL    string `json:"url"`
						Detail string `json:"detail,omitempty"`
					} `json:"image_url,omitempty"`
				}
				if m.Role != "user" || json.Unmarshal(m.Content, &parts) != nil || len(parts) == 0 || len(parts) > 64 {
					return in, errInput
				}
				for _, part := range parts {
					switch part.Type {
					case "text":
						if part.Text == "" || part.ImageURL != nil {
							return in, errInput
						}
					case "image_url":
						if part.ImageURL == nil || part.ImageURL.URL == "" || part.Text != "" {
							return in, errInput
						}
					default:
						return in, errInput
					}
				}
			}
		} else if len(m.ToolCalls) == 0 {
			return in, errInput
		}
		for _, call := range m.ToolCalls {
			var args map[string]any
			if call.ID == "" || call.Type != "function" || call.Function.Name == "" || json.Unmarshal([]byte(call.Function.Arguments), &args) != nil || args == nil {
				return in, errInput
			}
		}
	}
	for k := range in.Params {
		if !slices.Contains([]string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "seed", "reasoning_effort"}, k) {
			return in, errInput
		}
	}
	return in, nil
}

// Complete validates the transcript and sends the selected model request.
func (c *ChatClient) Complete(ctx context.Context, config catalog.ProviderConfig, key, model string, raw json.RawMessage) (Result, error) {
	in, err := parseChat(raw)
	if err != nil {
		return Result{}, err
	}
	return c.complete(ctx, config, key, model, in)
}
func (c *ChatClient) complete(ctx context.Context, config catalog.ProviderConfig, key, model string, in chatInput) (Result, error) {
	if config.Validate() != nil || !slices.Contains(config.Capabilities, "text") || model == "" || key == "" || !slices.Contains([]string{"deepseek", "openrouter", "openai", "vercel", "byteplus", "volcengine_ark"}, config.AdapterKey) {
		return Result{}, errInput
	}
	payload := map[string]any{"messages": in.Messages, "model": model, "stream": false}
	for k, v := range in.Params {
		payload[k] = v
	}
	if config.AdapterKey == "openai" {
		payload["max_completion_tokens"] = in.MaxTokens
	} else {
		payload["max_tokens"] = in.MaxTokens
	}
	if len(in.Tools) > 0 {
		list := make([]map[string]any, 0, len(in.Tools))
		for _, t := range in.Tools {
			list = append(list, map[string]any{"type": "function", "function": t})
		}
		payload["tools"] = list
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	total := 180 * time.Second
	if configured := time.Duration(config.TimeoutSeconds) * time.Second; configured > 0 && configured < total {
		total = configured
	}
	ctx, cancelTotal := context.WithTimeout(ctx, total)
	defer cancelTotal()
	ctx, cancelIdle := context.WithCancel(ctx)
	defer cancelIdle()
	watchdog := time.AfterFunc(c.idleTimeout, cancelIdle)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := c.client.Do(req)
	if err != nil {
		return Result{}, errResponse
	}
	// Closing the read-only response cannot change the received result.
	defer func() { _ = res.Body.Close() }()
	watchdog.Reset(c.idleTimeout)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Result{}, errResponse
	}
	const limit = 4 << 20
	data, err := io.ReadAll(io.LimitReader(progressReader{reader: res.Body, progress: func() { watchdog.Reset(c.idleTimeout) }}, limit+1))
	if err != nil || len(data) > limit {
		return Result{}, errResponse
	}
	var response struct {
		Choices []struct {
			Finish  string `json:"finish_reason"`
			Message struct {
				Content   *string    `json:"content"`
				ToolCalls []wireCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Input   *int64 `json:"prompt_tokens"`
			Output  *int64 `json:"completion_tokens"`
			Hit     *int64 `json:"prompt_cache_hit_tokens"`
			Details struct {
				Cached  *int64 `json:"cached_tokens"`
				Written *int64 `json:"cache_write_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.Choices) != 1 || response.Usage.Input == nil || response.Usage.Output == nil {
		return Result{}, errResponse
	}
	u := response.Usage
	out := Result{ModelKey: in.ModelKey, Usage: Usage{InputTokens: *u.Input, OutputTokens: *u.Output, CacheReadTokens: u.Details.Cached, CacheWriteTokens: u.Details.Written}}
	if u.Hit != nil {
		out.Usage.CacheReadTokens = u.Hit
	}
	if *u.Input < 0 || *u.Output < 0 || out.Usage.CacheReadTokens != nil && (*out.Usage.CacheReadTokens < 0 || *out.Usage.CacheReadTokens > *u.Input) || out.Usage.CacheWriteTokens != nil && *out.Usage.CacheWriteTokens < 0 {
		return Result{}, errResponse
	}
	choice := response.Choices[0]
	if choice.Message.Content != nil {
		out.Output = *choice.Message.Content
	}
	switch choice.Finish {
	case "tool_calls":
		out.StopReason = "tool_use"
	case "stop", "length":
		out.StopReason = choice.Finish
	default:
		return Result{}, errResponse
	}
	seen := map[string]bool{}
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "function" || call.ID == "" || seen[call.ID] || !slices.ContainsFunc(in.Tools, func(t tool) bool { return t.Name == call.Function.Name }) {
			return Result{}, errResponse
		}
		var args map[string]any
		dec := json.NewDecoder(strings.NewReader(call.Function.Arguments))
		dec.UseNumber()
		if !json.Valid([]byte(call.Function.Arguments)) || dec.Decode(&args) != nil || args == nil {
			return Result{}, errResponse
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{call.ID, call.Function.Name, args})
		seen[call.ID] = true
	}
	if (choice.Finish == "tool_calls") != (len(out.ToolCalls) > 0) {
		return Result{}, errResponse
	}
	return out, nil
}

// progressReader renews the idle window only after actual provider bytes.
type progressReader struct {
	reader   io.Reader
	progress func()
}

func (r progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.progress()
	}
	return n, err
}
