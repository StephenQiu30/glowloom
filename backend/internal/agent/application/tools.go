package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

var (
	ErrToolNotFound = errors.New("tool not found")
)

// ToolHandlerFunc processes a tool call against the agent state and returns raw JSON output.
type ToolHandlerFunc func(ctx context.Context, state *graph.AgentState, args json.RawMessage) (json.RawMessage, error)

// ToolMeta describes a registered tool and its operational classification.
type ToolMeta struct {
	Name            string
	Description     string
	IsWriteTool     bool
	IsProposalYield bool
	Handler         ToolHandlerFunc
}

// ToolRegistry maintains available tools for the LangGraph agent harness.
type ToolRegistry struct {
	tools       map[string]ToolMeta
	writeCoord  *WriteChainCoordinator
	turnTracker *TurnBudgetTracker
}

// NewToolRegistry creates a tool registry with write chain and budget coordination.
func NewToolRegistry(writeCoord *WriteChainCoordinator, tracker *TurnBudgetTracker) *ToolRegistry {
	if writeCoord == nil {
		writeCoord = NewWriteChainCoordinator()
	}
	return &ToolRegistry{
		tools:       make(map[string]ToolMeta),
		writeCoord:  writeCoord,
		turnTracker: tracker,
	}
}

// Register registers a tool definition.
func (r *ToolRegistry) Register(meta ToolMeta) {
	r.tools[meta.Name] = meta
}

// ExecuteToolCall executes one tool call against the current agent state.
func (r *ToolRegistry) ExecuteToolCall(ctx context.Context, state *graph.AgentState, call graph.ToolCall) graph.ToolResult {
	if r.turnTracker != nil {
		if err := r.turnTracker.RecordToolStep(0); err != nil {
			return graph.ToolResult{
				ToolCallID: call.ID,
				Name:       call.Name,
				Output:     json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error())),
				IsError:    true,
			}
		}
	}

	state.ToolStepCount++

	meta, ok := r.tools[call.Name]
	if !ok {
		return graph.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Output:     json.RawMessage(fmt.Sprintf(`{"error":%q}`, ErrToolNotFound.Error())),
			IsError:    true,
		}
	}

	output, err := meta.Handler(ctx, state, call.Arguments)
	if err != nil {
		return graph.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Output:     json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error())),
			IsError:    true,
		}
	}

	return graph.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Output:     output,
		IsError:    false,
	}
}

// ToolsNodeHandler implements graph.NodeHandler for the unified tools execution node.
func (r *ToolRegistry) ToolsNodeHandler(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
	if state == nil {
		return nil, errors.New("agent state cannot be nil")
	}

	// Find the latest message with pending tool calls
	if len(state.Messages) == 0 {
		return state, nil
	}

	latestMsg := &state.Messages[len(state.Messages)-1]
	if len(latestMsg.ToolCalls) == 0 {
		return state, nil
	}

	for _, call := range latestMsg.ToolCalls {
		result := r.ExecuteToolCall(ctx, state, call)

		// Append tool result message
		state.Messages = append(state.Messages, graph.Message{
			ID:         uuid.New(),
			Role:       "tool",
			Content:    string(result.Output),
			ToolResult: &result,
			Timestamp:  time.Now().UTC(),
		})

		// If a proposal yield was triggered, stop executing subsequent tool calls immediately (BeefTV Proposal Yield)
		if state.Interrupted {
			break
		}
	}

	return state, nil
}
