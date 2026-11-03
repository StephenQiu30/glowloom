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
	Status              string          `json:"status"` // "pending", "approved", "rejected"
	Approved            bool            `json:"approved"`
	CreatedAt           time.Time       `json:"created_at"`
}

// ProposalResolution communicates the user's approval/rejection when resuming the graph.
type ProposalResolution struct {
	ProposalID uuid.UUID `json:"proposal_id"`
	Approved   bool      `json:"approved"`
	Reason     string    `json:"reason,omitempty"`
}

// PlanStepState tracks the lifecycle of one step in the agent execution plan.
type PlanStepState struct {
	ID        string          `json:"id"`
	Agent     string          `json:"agent"` // "script_worker", "storyboard_worker", "shot_worker", "audio_worker"
	Title     string          `json:"title"`
	Status    string          `json:"status"` // "pending", "in_progress", "completed", "failed", "skipped"
	DependsOn []string        `json:"depends_on,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Result    string          `json:"result,omitempty"`
}

// ExecutionPlan holds the decomposed steps produced by the Supervisor.
type ExecutionPlan struct {
	Revision int64           `json:"revision"`
	Goal     string          `json:"goal"`
	Steps    []PlanStepState `json:"steps"`
	Approved bool            `json:"approved"`
}

// AgentState is the immutable-per-step state container passed along the LangGraph execution graph.
type AgentState struct {
	RunID           uuid.UUID       `json:"run_id"`
	ProjectID       uuid.UUID       `json:"project_id"`
	SessionID       uuid.UUID       `json:"session_id"`
	ActorID         uuid.UUID       `json:"actor_id"`
	CurrentRevision int64           `json:"current_revision"`
	WriteChain      WriteChainState `json:"write_chain"`

	Goal string         `json:"goal,omitempty"`
	Plan *ExecutionPlan `json:"plan,omitempty"`

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

// AddProposal appends a new proposal and immediately flags state interruption.
func (s *AgentState) AddProposal(p Proposal) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	s.Proposals = append(s.Proposals, p)
	s.Interrupt("proposal_approval_required")
}

// ResolveProposal updates a pending proposal with the user's decision.
func (s *AgentState) ResolveProposal(id uuid.UUID, approved bool) bool {
	for i := range s.Proposals {
		if s.Proposals[i].ID == id {
			s.Proposals[i].Approved = approved
			if approved {
				s.Proposals[i].Status = "approved"
			} else {
				s.Proposals[i].Status = "rejected"
			}
			return true
		}
	}
	return false
}

// PendingProposals returns any proposals awaiting approval.
func (s *AgentState) PendingProposals() []Proposal {
	var pending []Proposal
	for _, p := range s.Proposals {
		if p.Status == "pending" && !p.Approved {
			pending = append(pending, p)
		}
	}
	return pending
}

// NextReadyStep finds the next step whose dependencies are all completed.
func (s *AgentState) NextReadyStep() *PlanStepState {
	if s.Plan == nil {
		return nil
	}
	completed := make(map[string]bool)
	for _, step := range s.Plan.Steps {
		if step.Status == "completed" || step.Status == "skipped" {
			completed[step.ID] = true
		}
	}
	for i := range s.Plan.Steps {
		step := &s.Plan.Steps[i]
		if step.Status == "pending" {
			depsMet := true
			for _, dep := range step.DependsOn {
				if !completed[dep] {
					depsMet = false
					break
				}
			}
			if depsMet {
				return step
			}
		}
	}
	return nil
}

// StepByID returns a pointer to the plan step with matching ID.
func (s *AgentState) StepByID(id string) *PlanStepState {
	if s.Plan == nil {
		return nil
	}
	for i := range s.Plan.Steps {
		if s.Plan.Steps[i].ID == id {
			return &s.Plan.Steps[i]
		}
	}
	return nil
}

// MarkStepCompleted marks a step as completed and records its result.
func (s *AgentState) MarkStepCompleted(id string, result string) bool {
	step := s.StepByID(id)
	if step == nil {
		return false
	}
	step.Status = "completed"
	step.Result = result
	return true
}

// MarkStepFailed marks a step as failed.
func (s *AgentState) MarkStepFailed(id string, errReason string) bool {
	step := s.StepByID(id)
	if step == nil {
		return false
	}
	step.Status = "failed"
	step.Result = errReason
	return true
}

// AllStepsCompleted returns true if all steps in the plan are completed or skipped.
func (s *AgentState) AllStepsCompleted() bool {
	if s.Plan == nil || len(s.Plan.Steps) == 0 {
		return false
	}
	for _, step := range s.Plan.Steps {
		if step.Status != "completed" && step.Status != "skipped" {
			return false
		}
	}
	return true
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
