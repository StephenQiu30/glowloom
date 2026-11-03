package agent_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestWriteSSEEventFormatting(t *testing.T) {
	var buf bytes.Buffer
	runID := uuid.New()
	proposalID := uuid.New()

	event := agenthttp.AgentStreamEvent{
		ID:       "1",
		Type:     agenthttp.EventProposal,
		RunID:    runID,
		NodeName: "shot_worker",
		Data: agenthttp.ProposalCardPayload{
			ProposalID:          proposalID,
			Action:              "shot.video.generate",
			Summary:             "生成镜头1超清视频",
			EstimatedCostMicros: 5000000,
			Data:                json.RawMessage(`{"prompt":"雨夜霓虹"}`),
			ExpiresAt:           time.Now().Add(1 * time.Hour),
		},
		Timestamp: time.Now().UTC(),
	}

	err := agenthttp.WriteSSEEvent(&buf, event)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "id: 1\n") {
		t.Errorf("expected id: 1, got %s", output)
	}
	if !strings.Contains(output, "event: proposal\n") {
		t.Errorf("expected event: proposal, got %s", output)
	}
	if !strings.Contains(output, "data: {") {
		t.Errorf("expected data json, got %s", output)
	}
	if !strings.HasSuffix(output, "\n\n") {
		t.Errorf("SSE frame must terminate with double newline, got %q", output[len(output)-2:])
	}
}

func TestStreamHandlerServeEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runID := uuid.New()

	handler := agenthttp.NewStreamHandler(func(ctx *gin.Context, rid uuid.UUID) (<-chan agenthttp.AgentStreamEvent, error) {
		ch := make(chan agenthttp.AgentStreamEvent, 3)
		ch <- agenthttp.AgentStreamEvent{
			ID:    "1",
			Type:  agenthttp.EventThinking,
			RunID: rid,
			Data:  map[string]string{"delta": "正在分析剧本场景..."},
		}
		ch <- agenthttp.AgentStreamEvent{
			ID:    "2",
			Type:  agenthttp.EventText,
			RunID: rid,
			Data:  map[string]string{"delta": "已完成第1阶段。"},
		}
		ch <- agenthttp.AgentStreamEvent{
			ID:    "3",
			Type:  agenthttp.EventTurnEnd,
			RunID: rid,
			Data:  map[string]string{"reason": "finished"},
		}
		close(ch)
		return ch, nil
	})

	router := gin.New()
	router.GET("/api/projects/:pid/agent-runs/:rid/events", handler.ServeEvents)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+uuid.New().String()+"/agent-runs/"+runID.String()+"/events", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("expected text/event-stream content type, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: thinking") {
		t.Errorf("expected event thinking in body, got %s", body)
	}
	if !strings.Contains(body, "event: text") {
		t.Errorf("expected event text in body, got %s", body)
	}
	if !strings.Contains(body, "event: turn_end") {
		t.Errorf("expected event turn_end in body, got %s", body)
	}
}
