package operation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
)

type chatLauncher struct {
	t        *testing.T
	behavior string
	turns    int
	threads  int
}

func (l *chatLauncher) Open(_ context.Context, _ string) (io.ReadWriteCloser, error) {
	c := &codexTestConn{messages: make(chan []byte, 32), done: make(chan struct{})}
	c.respond = func(raw []byte) {
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		require.NoError(l.t, json.Unmarshal(raw, &req))
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"userAgent": "fixture"}
		case "initialized":
			return
		case "account/read":
			result = map[string]any{"account": map[string]any{"type": "chatgpt", "email": "must-not-escape@example.test"}}
			if l.behavior == "logged-out" {
				result = map[string]any{"account": nil}
			}
		case "modelProvider/capabilities/read":
			result = map[string]any{"imageGeneration": true}
		case "model/list":
			result = map[string]any{"data": []any{map[string]any{"model": "fixture-chat", "hidden": false}}, "nextCursor": nil}
		case "thread/start":
			l.threads++
			require.Equal(l.t, "never", req.Params["approvalPolicy"])
			require.Equal(l.t, "read-only", req.Params["sandbox"])
			require.Equal(l.t, false, req.Params["allowProviderModelFallback"])
			result = map[string]any{"model": "fixture-chat", "thread": map[string]any{"id": "owned-thread"}}
		case "turn/start":
			l.turns++
			require.Equal(l.t, "owned-thread", req.Params["threadId"])
			turn := fmt.Sprint("turn-", l.turns)
			c.emit(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]any{"id": turn}}})
			if l.behavior == "eof" {
				close(c.messages)
				return
			}
			emit := func(method string, p map[string]any) { c.emit(map[string]any{"method": method, "params": p}) }
			emit("item/completed", map[string]any{"threadId": "foreign", "turnId": turn, "item": map[string]any{"id": "foreign", "type": "agentMessage", "text": "foreign text"}})
			emit("item/agentMessage/delta", map[string]any{"threadId": "owned-thread", "turnId": turn, "delta": "answer"})
			kind := "agentMessage"
			if l.behavior == "tool" {
				kind = "commandExecution"
			}
			emit("item/completed", map[string]any{"threadId": "owned-thread", "turnId": turn, "item": map[string]any{"id": "item-" + turn, "type": kind, "text": "answer " + turn, "phase": "final_answer"}})
			state := "completed"
			if l.behavior == "failed" {
				state = "failed"
			}
			emit("turn/completed", map[string]any{"threadId": "owned-thread", "turn": map[string]any{"id": turn, "status": state}})
			return
		default:
			l.t.Fatalf("unexpected RPC %s", req.Method)
		}
		c.emit(map[string]any{"id": req.ID, "result": result})
	}
	return c, nil
}
func TestCodexChatKeepsThreeTurnsOnOnePrivateThread(t *testing.T) {
	launcher := &chatLauncher{t: t}
	session, err := codex.OpenChat(t.Context(), launcher, t.TempDir(), "fixture-chat")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	for i := 1; i <= 3; i++ {
		delta := ""
		result, err := session.Turn(t.Context(), "hello", func(text string) { delta += text })
		require.NoError(t, err)
		require.Equal(t, "completed", result.Outcome)
		require.Equal(t, fmt.Sprint("answer turn-", i), result.Text)
		require.Equal(t, "answer", delta)
		require.Nil(t, result.Usage)
	}
	require.Equal(t, 1, launcher.threads)
	require.Equal(t, 3, launcher.turns)
}
func TestCodexChatUnknownNeverResends(t *testing.T) {
	for _, behavior := range []string{"eof", "tool", "failed"} {
		t.Run(behavior, func(t *testing.T) {
			launcher := &chatLauncher{t: t, behavior: behavior}
			session, err := codex.OpenChat(t.Context(), launcher, t.TempDir(), "fixture-chat")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, session.Close()) })
			result, err := session.Turn(t.Context(), "hello", nil)
			require.NoError(t, err)
			require.Equal(t, "unknown", result.Outcome)
			result, err = session.Turn(t.Context(), "hello", nil)
			require.NoError(t, err)
			require.Equal(t, "not_submitted", result.Outcome)
			require.Equal(t, 1, launcher.turns)
		})
	}
}
func TestCodexProbeDoesNotExposeAccountOrSubmitInference(t *testing.T) {
	launcher := &chatLauncher{t: t}
	status, err := codex.Probe(t.Context(), launcher, t.TempDir())
	require.NoError(t, err)
	require.True(t, status.Authenticated)
	data, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(data), "email")
	require.Zero(t, launcher.threads)
	require.Zero(t, launcher.turns)
	launcher.behavior = "logged-out"
	_, err = codex.OpenChat(t.Context(), launcher, t.TempDir(), "fixture-chat")
	require.Error(t, err)
	require.Zero(t, launcher.threads)
}
