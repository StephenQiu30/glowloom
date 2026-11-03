package graph

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// WriteChainState tracks sequential in-turn revisions to prevent self-CAS collisions.
type WriteChainState struct {
	Head             int64 `json:"head"`
	Start            int64 `json:"start"`
	ExpectedRevision int64 `json:"expected_revision"`
}

// Extend advances the local revision chain for safe chained multi-step tool calls.
func (w *WriteChainState) Extend(expectedRevision, newRevision int64) bool {
	if newRevision <= expectedRevision {
		return false
	}
	if w.Head == 0 || expectedRevision == w.Head {
		if w.Start == 0 {
			w.Start = expectedRevision
		}
		w.Head = newRevision
		w.ExpectedRevision = newRevision
		return true
	}
	// Branch conflict outside current chain
	return false
}

// ToolCall represents one invocation requested by the model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult represents the outcome of a tool execution.
type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Output     json.RawMessage `json:"output"`
	IsError    bool            `json:"is_error"`
}

// Message is an entry in the agent conversation history.
type Message struct {
	ID         uuid.UUID   `json:"id"`
	Role       string      `json:"role"` // "system", "user", "assistant", "tool"
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}

// Proposal captures an action requiring explicit user approval (Proposal Yield).
type Proposal struct {
	ID                  uuid.UUID       `json:"id"`
	Action              string          `json:"action"` // e.g. "generation.submit", "canvas.modify"
	Summary             string          `json:"summary"`
	EstimatedCostMicros int64           `json:"estimated_cost_micros"`
	Data                json.RawMessage `json:"data"`
	Approved            bool            `json:"approved"`
	CreatedAt           time.Time       `json:"created_at"`
}

// AgentState is the immutable-per-step state container passed along the LangGraph execution graph.
type AgentState struct {
	RunID           uuid.UUID       `json:"run_id"`
	ProjectID       uuid.UUID       `json:"project_id"`
	SessionID       uuid.UUID       `json:"session_id"`
	ActorID         uuid.UUID       `json:"actor_id"`
	CurrentRevision int64           `json:"current_revision"`
	WriteChain      WriteChainState `json:"write_chain"`

	Messages     []Message  `json:"messages"`
	Proposals    []Proposal `json:"proposals"`
	LoadedSkills []string   `json:"loaded_skills"`

	LastStudioHash string `json:"last_studio_hash"`
	StudioDiffText string `json:"studio_diff_text,omitempty"`

	TurnStepCount int `json:"turn_step_count"`
	ToolStepCount int `json:"tool_step_count"`
	MaxTurnSteps  int `json:"max_turn_steps"`

	Status          string    `json:"status"` // "running", "suspended_for_approval", "completed", "failed"
	Interrupted     bool      `json:"interrupted"`
	InterruptReason string    `json:"interrupt_reason,omitempty"`
	CurrentNode     string    `json:"current_node,omitempty"`
	CheckpointID    uuid.UUID `json:"checkpoint_id,omitempty"`
}

// Interrupt flags the graph to yield immediately (Proposal Yield), save state, and await human approval.
func (s *AgentState) Interrupt(reason string) {
	s.Interrupted = true
	s.InterruptReason = reason
	s.Status = "suspended_for_approval"
}

// Checkpoint represents a persisted snapshot of the state graph at a node boundary.
type Checkpoint struct {
	RunID        uuid.UUID   `json:"run_id"`
	CheckpointID uuid.UUID   `json:"checkpoint_id"`
	NodeName     string      `json:"node_name"`
	State        *AgentState `json:"state"`
	CreatedAt    time.Time   `json:"created_at"`
}

// Checkpointer abstracts storage of graph execution checkpoints.
type Checkpointer interface {
	SaveCheckpoint(ctx context.Context, runID, checkpointID uuid.UUID, nodeName string, state *AgentState) error
	GetLatestCheckpoint(ctx context.Context, runID uuid.UUID) (*Checkpoint, error)
}
