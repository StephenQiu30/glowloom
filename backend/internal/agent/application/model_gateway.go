package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

var (
	ErrModelNotFound      = errors.New("model not found or unsupported")
	ErrContextExceeded    = errors.New("input tokens exceed model context window")
	ErrMaxTokensInvalid   = errors.New("max tokens exceed model limit")
	ErrStreamStalled      = errors.New("model stream stalled: chunk idle timeout")
	ErrTurnBudgetExceeded = errors.New("turn budget exceeded")
	ErrBudgetOverdraft    = errors.New("cost exceeds reserved budget")
)

// ModelRequest contains immutable frozen model invocation parameters.
type ModelRequest struct {
	Model       domain.DialogueModel
	Messages    []graph.Message
	Tools       []ToolDefinition
	MaxTokens   int
	Temperature float32
}

// ToolDefinition describes a callable function for model tool-use reasoning.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// StreamChunk conveys one streaming token, thinking delta, or tool invocation chunk.
type StreamChunk struct {
	Type         string          `json:"type"` // "thinking", "text", "tool_call", "usage"
	DeltaText    string          `json:"delta_text,omitempty"`
	ThinkingText string          `json:"thinking_text,omitempty"`
	ToolCall     *graph.ToolCall `json:"tool_call,omitempty"`
	InputTokens  int             `json:"input_tokens,omitempty"`
	OutputTokens int             `json:"output_tokens,omitempty"`
	CostMicros   int64           `json:"cost_micros,omitempty"`
	Error        error           `json:"-"`
}

// ModelResponse captures the complete, validated response from a model invocation.
type ModelResponse struct {
	Content      string           `json:"content"`
	Thinking     string           `json:"thinking,omitempty"`
	ToolCalls    []graph.ToolCall `json:"tool_calls,omitempty"`
	InputTokens  int              `json:"input_tokens"`
	OutputTokens int              `json:"output_tokens"`
	CostMicros   int64            `json:"cost_micros"`
}

// ModelGateway provides unified model access with parameter freezing and token budget checks.
type ModelGateway interface {
	Generate(ctx context.Context, req ModelRequest) (*ModelResponse, error)
	Stream(ctx context.Context, req ModelRequest) (<-chan StreamChunk, error)
}

// DefaultModelGateway adapts Go catalog and enforces parameter freezing and context boundaries.
type DefaultModelGateway struct {
	generator func(ctx context.Context, req ModelRequest) (*ModelResponse, error)
	streamer  func(ctx context.Context, req ModelRequest) (<-chan StreamChunk, error)
	watchdog  *StreamWatchdog
}

// NewDefaultModelGateway creates a ModelGateway with the configured generator and streaming watchdog.
func NewDefaultModelGateway(
	gen func(ctx context.Context, req ModelRequest) (*ModelResponse, error),
	stream func(ctx context.Context, req ModelRequest) (<-chan StreamChunk, error),
	watchdogTimeout time.Duration,
) *DefaultModelGateway {
	return &DefaultModelGateway{
		generator: gen,
		streamer:  stream,
		watchdog:  NewStreamWatchdog(watchdogTimeout),
	}
}

// ValidateRequest freezes parameters and ensures context window compliance.
func ValidateRequest(req ModelRequest) error {
	if req.Model.ReleaseID == uuid.Nil {
		return ErrModelNotFound
	}
	if req.Model.ContextWindow < 1024 {
		return ErrModelNotFound
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = req.Model.MaxTokens
	}
	if req.MaxTokens >= req.Model.ContextWindow {
		return ErrMaxTokensInvalid
	}

	// Approximate input token count (4 chars ~ 1 token heuristic for preflight check)
	estimatedInputTokens := 0
	for _, msg := range req.Messages {
		estimatedInputTokens += len(msg.Content)/4 + 4
	}
	if estimatedInputTokens+req.MaxTokens > req.Model.ContextWindow {
		return fmt.Errorf("%w: estimated %d + max %d > context %d",
			ErrContextExceeded, estimatedInputTokens, req.MaxTokens, req.Model.ContextWindow)
	}

	return nil
}

func (g *DefaultModelGateway) Generate(ctx context.Context, req ModelRequest) (*ModelResponse, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}
	if g.generator == nil {
		return nil, errors.New("generator not implemented")
	}
	return g.generator(ctx, req)
}

func (g *DefaultModelGateway) Stream(ctx context.Context, req ModelRequest) (<-chan StreamChunk, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}
	if g.streamer == nil {
		return nil, errors.New("streamer not implemented")
	}

	rawStream, err := g.streamer(ctx, req)
	if err != nil {
		return nil, err
	}

	if g.watchdog != nil {
		return g.watchdog.Watch(ctx, rawStream), nil
	}
	return rawStream, nil
}
