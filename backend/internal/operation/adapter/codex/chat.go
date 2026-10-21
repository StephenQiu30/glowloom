package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// RuntimeStatus contains public capability facts, never account identifiers or
// credentials. Discovery is not proof that a paid inference will succeed.
type RuntimeStatus struct {
	Authenticated   bool     `json:"authenticated"`
	ImageGeneration bool     `json:"image_generation"`
	Models          []string `json:"models"`
}

// Probe performs a non-inference handshake in a fresh task directory.
func Probe(ctx context.Context, launcher Launcher, workRoot string) (RuntimeStatus, error) {
	if launcher == nil || !filepath.IsAbs(workRoot) {
		return RuntimeStatus{}, application.ErrInvalidProviderCall
	}
	dir, err := os.MkdirTemp(workRoot, "probe-")
	if err != nil {
		return RuntimeStatus{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	conn, err := launcher.Open(ctx, dir)
	if err != nil {
		return RuntimeStatus{}, err
	}
	client := newRPC(conn)
	defer client.close()
	return inspectRuntime(ctx, client)
}
func inspectRuntime(ctx context.Context, client *rpcClient) (RuntimeStatus, error) {
	status := RuntimeStatus{Models: []string{}}
	var initialized struct {
		UserAgent string `json:"userAgent"`
	}
	if err := client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]any{"name": "glowloom-runtime", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}}, &initialized); err != nil {
		return status, err
	}
	if initialized.UserAgent == "" {
		return status, errProtocol
	}
	if err := client.send(ctx, map[string]any{"method": "initialized"}); err != nil {
		return status, err
	}
	var account struct {
		Account json.RawMessage `json:"account"`
	}
	if err := client.call(ctx, "account/read", map[string]any{"refreshToken": false}, &account); err != nil {
		return status, err
	}
	status.Authenticated = len(account.Account) > 0 && string(account.Account) != "null"
	var caps struct {
		ImageGeneration bool `json:"imageGeneration"`
	}
	if err := client.call(ctx, "modelProvider/capabilities/read", map[string]any{}, &caps); err != nil {
		return status, err
	}
	status.ImageGeneration = caps.ImageGeneration
	var cursor *string
	for page := 0; page < 5; page++ {
		var models struct {
			Data []struct {
				Model  string `json:"model"`
				Hidden bool   `json:"hidden"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := client.call(ctx, "model/list", map[string]any{"limit": 100, "cursor": cursor, "includeHidden": false}, &models); err != nil {
			return status, err
		}
		for _, model := range models.Data {
			if !model.Hidden && validID(model.Model) {
				status.Models = append(status.Models, model.Model)
			}
		}
		if models.NextCursor == nil {
			return status, nil
		}
		cursor = models.NextCursor
	}
	return RuntimeStatus{}, errProtocol
}

// ChatSession is a private live transport, not a public conversation identifier.
// The application must persist its operation/turn identity before calling Turn.
// Unknown delivery poisons the session so a reconnect cannot silently resend.
type ChatSession struct {
	mu                 sync.Mutex
	client             *rpcClient
	dir, model, thread string
	uncertain, closed  bool
}

// OpenChat fixes the model and opens one read-only thread. It never accepts a
// caller-supplied upstream thread ID or falls back to another model.
func OpenChat(ctx context.Context, launcher Launcher, root, model string) (*ChatSession, error) {
	return openChat(ctx, launcher, root, model, "")
}

// ResumeChat resumes only a server-owned thread stored with a completed trial.
func ResumeChat(ctx context.Context, launcher Launcher, root, model, thread string) (*ChatSession, error) {
	if !validID(thread) {
		return nil, application.ErrInvalidProviderCall
	}
	return openChat(ctx, launcher, root, model, thread)
}

func openChat(ctx context.Context, launcher Launcher, root, model, thread string) (*ChatSession, error) {
	if launcher == nil || !filepath.IsAbs(root) || !validID(model) {
		return nil, application.ErrInvalidProviderCall
	}
	dir, err := os.MkdirTemp(root, "chat-")
	if err != nil {
		return nil, err
	}
	conn, err := launcher.Open(ctx, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	client := newRPC(conn)
	failed := true
	defer func() {
		if failed {
			client.close()
			_ = os.RemoveAll(dir)
		}
	}()
	status, err := inspectRuntime(ctx, client)
	if err != nil {
		return nil, err
	}
	available := false
	for _, id := range status.Models {
		if id == model {
			available = true
		}
	}
	if !status.Authenticated || !available {
		return nil, fmt.Errorf("codex chat model unavailable")
	}
	var response struct {
		Model  string `json:"model"`
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	params := map[string]any{"model": model, "cwd": dir, "sandbox": "read-only", "approvalPolicy": "never", "allowProviderModelFallback": false, "dynamicTools": []any{}, "selectedCapabilityRoots": []string{}, "developerInstructions": "Answer the user's messages as a text chat assistant. Do not use tools, access files, execute commands, or delegate work.", "config": map[string]any{"web_search": "disabled", "features.image_generation": false}}
	method := "thread/start"
	if thread != "" {
		method = "thread/resume"
		params["threadId"] = thread
	}
	if err := client.call(ctx, method, params, &response); err != nil {
		return nil, err
	}
	if response.Model != model || !validID(response.Thread.ID) || (thread != "" && response.Thread.ID != thread) {
		return nil, errProtocol
	}
	failed = false
	return &ChatSession{client: client, dir: dir, model: model, thread: response.Thread.ID}, nil
}

// ThreadID is internal runtime identity; public APIs must not accept it from callers.
func (s *ChatSession) ThreadID() string { return s.thread }

// Turn serializes conversation turns. Deltas are provisional and must never be
// treated as a final answer or a billing receipt by the caller.
func (s *ChatSession) Turn(ctx context.Context, prompt string, delta func(string)) (application.GenerationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.uncertain || strings.TrimSpace(prompt) == "" || len(prompt) > 32*1024 || ctx.Err() != nil {
		return application.GenerationResult{Outcome: "not_submitted", ErrorCode: "codex_chat_unavailable"}, nil
	}
	s.uncertain = true
	turnCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ctx = turnCtx
	turn, err := startTurn(ctx, s.client, s.thread, s.model, prompt)
	if err != nil {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "codex_turn_delivery_uncertain"}, nil
	}
	text, err := completedText(ctx, s.client, s.thread, turn, delta)
	if err != nil {
		s.client.close()
		s.closed = true
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "codex_chat_completion_uncertain"}, nil
	}
	s.uncertain = false
	return application.GenerationResult{Outcome: "completed", State: "succeeded", Text: text}, nil
}

// Close terminates the child and removes scratch files. It never sends a turn.
func (s *ChatSession) Close() error {
	s.client.close()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client.close()
	s.closed = true
	return os.RemoveAll(s.dir)
}
func completedText(ctx context.Context, client *rpcClient, thread, turn string, delta func(string)) (string, error) {
	final := map[string]string{}
	order := []string{}
	total := 0
	streamed := 0
	for {
		event, err := client.event(ctx)
		if err != nil {
			return "", err
		}
		var n struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Delta    string `json:"delta"`
			Item     struct {
				ID    string `json:"id"`
				Type  string `json:"type"`
				Text  string `json:"text"`
				Phase string `json:"phase"`
			} `json:"item"`
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"turn"`
		}
		if decodeJSON(event.Params, &n) != nil {
			return "", errProtocol
		}
		if n.ThreadID != thread {
			continue
		}
		if event.Method == "turn/completed" {
			if n.Turn.ID != turn {
				continue
			}
			if n.Turn.Status != "completed" || len(order) == 0 {
				return "", errProtocol
			}
			parts := make([]string, 0, len(order))
			for _, id := range order {
				parts = append(parts, final[id])
			}
			return strings.Join(parts, "\n"), nil
		}
		if n.TurnID != turn {
			continue
		}
		switch event.Method {
		case "item/agentMessage/delta":
			streamed += len(n.Delta)
			if streamed > 1<<20 {
				return "", errProtocol
			}
			if delta != nil {
				delta(n.Delta)
			}
		case "item/completed":
			switch n.Item.Type {
			case "userMessage", "reasoning":
				continue
			case "agentMessage":
				if n.Item.Phase != "" && n.Item.Phase != "final_answer" {
					continue
				}
				if !validID(n.Item.ID) || strings.TrimSpace(n.Item.Text) == "" {
					return "", errProtocol
				}
				if previous, ok := final[n.Item.ID]; ok {
					if previous != n.Item.Text {
						return "", errProtocol
					}
					continue
				}
				total += len(n.Item.Text)
				if total > 1<<20 || len(order) >= 64 {
					return "", errProtocol
				}
				final[n.Item.ID] = n.Item.Text
				order = append(order, n.Item.ID)
			default:
				return "", errProtocol
			}
		}
	}
}
