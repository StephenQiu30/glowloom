package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AgentEventType defines supported SSE event names.
type AgentEventType string

const (
	EventThinking   AgentEventType = "thinking"
	EventText       AgentEventType = "text"
	EventToolCall   AgentEventType = "tool_call"
	EventToolResult AgentEventType = "tool_result"
	EventProposal   AgentEventType = "proposal"
	EventTurnEnd    AgentEventType = "turn_end"
)

// AgentStreamEvent is the structured payload for an agent SSE frame.
type AgentStreamEvent struct {
	ID        string         `json:"id"`
	Type      AgentEventType `json:"type"`
	RunID     uuid.UUID      `json:"run_id"`
	NodeName  string         `json:"node_name,omitempty"`
	Data      any            `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
}

// WriteSSEEvent formats and writes a single SSE message frame to the writer.
func WriteSSEEvent(w io.Writer, event AgentStreamEvent) error {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, raw)
	return err
}

// StreamHandler serves real-time agent execution events over Server-Sent Events (SSE).
type StreamHandler struct {
	eventSource func(ctx *gin.Context, runID uuid.UUID) (<-chan AgentStreamEvent, error)
}

// NewStreamHandler creates an SSE stream handler with the given event supplier.
func NewStreamHandler(src func(ctx *gin.Context, runID uuid.UUID) (<-chan AgentStreamEvent, error)) *StreamHandler {
	return &StreamHandler{eventSource: src}
}

// ServeEvents handles GET /api/projects/:pid/agent-runs/:rid/events
func (h *StreamHandler) ServeEvents(c *gin.Context) {
	runIDStr := c.Param("rid")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid run_id"})
		return
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	events, err := h.eventSource(c, runID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := WriteSSEEvent(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// ProposalCardPayload is the frontend structure emitted in "proposal" SSE event.
type ProposalCardPayload struct {
	ProposalID          uuid.UUID       `json:"proposal_id"`
	Action              string          `json:"action"`
	Summary             string          `json:"summary"`
	EstimatedCostMicros int64           `json:"estimated_cost_micros"`
	Data                json.RawMessage `json:"data"`
	ExpiresAt           time.Time       `json:"expires_at"`
}
