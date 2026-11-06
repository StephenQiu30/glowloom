// Code generated from agent/contracts TypeBox schemas and route metadata. DO NOT EDIT.
package runtimeclient

import "encoding/json"

type AuthorizedInput struct {
	SchemaVersion         float64         `json:"schema_version" binding:"required"`
	ReferencesUnavailable *bool           `json:"references_unavailable,omitempty"`
	Draft                 json.RawMessage `json:"draft" swaggertype:"object" binding:"required"`
	InputHash             string          `json:"input_hash" binding:"required"`
	PlainText             string          `json:"plain_text" binding:"required"`
	Prompt                string          `json:"prompt" binding:"required"`
	MediaPins             []struct {
		SourceId      string  `json:"sourceId" binding:"required"`
		SourceVersion string  `json:"sourceVersion" binding:"required"`
		MediaKind     string  `json:"mediaKind" binding:"required"`
		BindingId     *string `json:"bindingId,omitempty"`
	} `json:"media_pins" binding:"required"`
	Quote *struct {
		RunID       string `json:"run_id" binding:"required"`
		MessageID   string `json:"message_id" binding:"required"`
		Text        string `json:"text" binding:"required"`
		ContentHash string `json:"content_hash" binding:"required"`
	} `json:"quote,omitempty"`
}

type CreateSessionRequest struct {
	ActorID     string  `json:"actor_id" binding:"required"`
	OrgID       string  `json:"org_id" binding:"required"`
	ScopeID     string  `json:"scope_id" binding:"required"`
	ProjectID   string  `json:"project_id" binding:"required"`
	CanvasID    *string `json:"canvas_id,omitempty"`
	RequestID   string  `json:"request_id" binding:"required"`
	ContentHash string  `json:"content_hash" binding:"required"`
	Title       string  `json:"title" binding:"required"`
}

type UpdateSessionRequest struct {
	ActorID   string  `json:"actor_id" binding:"required"`
	OrgID     string  `json:"org_id" binding:"required"`
	ScopeID   string  `json:"scope_id" binding:"required"`
	ProjectID string  `json:"project_id" binding:"required"`
	CanvasID  *string `json:"canvas_id,omitempty"`
	Action    string  `json:"action" binding:"required"`
	Title     *string `json:"title,omitempty"`
}

type CreateRunRequest struct {
	ActorID                   string          `json:"actor_id" binding:"required"`
	OrgID                     string          `json:"org_id" binding:"required"`
	ScopeID                   string          `json:"scope_id" binding:"required"`
	ProjectID                 string          `json:"project_id" binding:"required"`
	CanvasID                  *string         `json:"canvas_id,omitempty"`
	RequestID                 string          `json:"request_id" binding:"required"`
	ContentHash               string          `json:"content_hash" binding:"required"`
	ExpectedExecutionRevision int64           `json:"expected_execution_revision" binding:"required"`
	Goal                      string          `json:"goal" binding:"required"`
	Input                     AuthorizedInput `json:"input" binding:"required"`
	RetryOfRunID              *string         `json:"retry_of_run_id,omitempty"`
	Manifest                  json.RawMessage `json:"manifest" swaggertype:"object" binding:"required"`
}

type ControlRequest struct {
	ActorID               string  `json:"actor_id" binding:"required"`
	OrgID                 string  `json:"org_id" binding:"required"`
	ScopeID               string  `json:"scope_id" binding:"required"`
	ProjectID             string  `json:"project_id" binding:"required"`
	CanvasID              *string `json:"canvas_id,omitempty"`
	DecisionID            string  `json:"decision_id" binding:"required"`
	RootRunID             string  `json:"root_run_id" binding:"required"`
	SessionID             string  `json:"session_id" binding:"required"`
	AuthorizationRevision int64   `json:"authorization_revision" binding:"required"`
	Action                string  `json:"action" binding:"required"`
	ContentHash           string  `json:"content_hash" binding:"required"`
}

type SessionView struct {
	ActorID           string       `json:"actor_id" binding:"required"`
	OrgID             string       `json:"org_id" binding:"required"`
	ScopeID           string       `json:"scope_id" binding:"required"`
	ProjectID         string       `json:"project_id" binding:"required"`
	CanvasID          *string      `json:"canvas_id,omitempty"`
	ID                string       `json:"id" binding:"required"`
	Title             string       `json:"title" binding:"required"`
	Pinned            *bool        `json:"pinned,omitempty"`
	ExecutionRevision int64        `json:"execution_revision" binding:"required"`
	CreatedAt         *string      `json:"created_at,omitempty"`
	Run               *RunSnapshot `json:"run,omitempty"`
}

type RunSnapshot struct {
	ExecutionAvailable bool             `json:"execution_available" binding:"required"`
	HistoryIncomplete  *bool            `json:"history_incomplete,omitempty"`
	MessagesPage       MessagePage      `json:"messages_page" binding:"required"`
	RootRunID          string           `json:"root_run_id" binding:"required"`
	SessionID          string           `json:"session_id" binding:"required"`
	SubmissionID       string           `json:"submission_id" binding:"required"`
	Goal               string           `json:"goal" binding:"required"`
	Input              *AuthorizedInput `json:"input,omitempty"`
	RetryOfRunID       *string          `json:"retry_of_run_id,omitempty"`
	Manifest           json.RawMessage  `json:"manifest" swaggertype:"object" binding:"required"`
	State              AgentState       `json:"state" binding:"required"`
	SnapshotCursor     string           `json:"snapshot_cursor" binding:"required"`
	Events             []struct {
		EventType string          `json:"event_type" binding:"required"`
		Payload   json.RawMessage `json:"payload" swaggertype:"object" binding:"required"`
	} `json:"events" binding:"required"`
}

type AgentState struct {
	Status                string      `json:"status" binding:"required"`
	ExecutionRevision     int64       `json:"execution_revision" binding:"required"`
	AuthorizationRevision *int64      `json:"authorization_revision,omitempty"`
	RootRunID             *string     `json:"root_run_id,omitempty"`
	SegmentID             *string     `json:"segment_id,omitempty"`
	ModelSteps            *int64      `json:"model_steps,omitempty"`
	ToolSteps             *int64      `json:"tool_steps,omitempty"`
	Goal                  *string     `json:"goal,omitempty"`
	Plan                  *[]PlanItem `json:"plan,omitempty"`
	Delegations           *[]struct {
		TaskID         string    `json:"task_id" binding:"required"`
		ConversationID int64     `json:"conversation_id" binding:"required"`
		Role           string    `json:"role" binding:"required"`
		Status         string    `json:"status" binding:"required"`
		SourceRefs     *[]string `json:"source_refs,omitempty"`
		Adopted        *bool     `json:"adopted,omitempty"`
		ModelBudget    *int64    `json:"model_budget,omitempty"`
		ToolBudget     *int64    `json:"tool_budget,omitempty"`
		ModelUsed      *int64    `json:"model_used,omitempty"`
		ToolUsed       *int64    `json:"tool_used,omitempty"`
	} `json:"delegations,omitempty"`
	Adopted         *[]json.RawMessage `json:"adopted,omitempty" swaggertype:"array,object"`
	PendingProposal *string            `json:"pending_proposal,omitempty" extensions:"x-nullable"`
	Proposal        *ProposalView      `json:"proposal,omitempty"`
	Reconciliation  *json.RawMessage   `json:"reconciliation,omitempty" swaggertype:"object"`
	Failure         *json.RawMessage   `json:"failure,omitempty" swaggertype:"object"`
}

type PlanItem struct {
	ID        string    `json:"id" binding:"required"`
	Title     string    `json:"title" binding:"required"`
	Status    string    `json:"status" binding:"required"`
	DependsOn *[]string `json:"depends_on,omitempty"`
	Artifacts *[]string `json:"artifacts,omitempty"`
}

type ProposalView struct {
	ProposalID            string          `json:"proposal_id" binding:"required"`
	CandidateHash         string          `json:"candidate_hash" binding:"required"`
	AuthorizationRevision int64           `json:"authorization_revision" binding:"required"`
	Candidate             json.RawMessage `json:"candidate" swaggertype:"object" binding:"required"`
	CanvasRevision        *int64          `json:"canvas_revision,omitempty"`
	PriceVersion          *string         `json:"price_version,omitempty"`
	QuoteMicros           *int64          `json:"quote_micros,omitempty"`
	BudgetMicros          *int64          `json:"budget_micros,omitempty"`
}

type AcceptedRun struct {
	RootRunID         string `json:"root_run_id" binding:"required"`
	SessionID         string `json:"session_id" binding:"required"`
	SubmissionID      string `json:"submission_id" binding:"required"`
	Status            string `json:"status" binding:"required"`
	ExecutionRevision int64  `json:"execution_revision" binding:"required"`
}

type ControlReceipt struct {
	DecisionID            string `json:"decision_id" binding:"required"`
	AuthorizationRevision int64  `json:"authorization_revision" binding:"required"`
	Status                string `json:"status" binding:"required"`
	ExecutionRevision     int64  `json:"execution_revision" binding:"required"`
	ReceiptID             string `json:"receipt_id" binding:"required"`
}

type SessionPage struct {
	Items      []SessionView `json:"items" binding:"required"`
	NextCursor *string       `json:"next_cursor,omitempty"`
}

type RunPage struct {
	Items []struct {
		RootRunID        string  `json:"root_run_id" binding:"required"`
		SessionID        string  `json:"session_id" binding:"required"`
		AcceptedRevision int64   `json:"accepted_revision" binding:"required"`
		CreatedAt        string  `json:"created_at" binding:"required"`
		Status           string  `json:"status" binding:"required"`
		InputExcerpt     string  `json:"input_excerpt" binding:"required"`
		RetryOfRunID     *string `json:"retry_of_run_id,omitempty"`
	} `json:"items" binding:"required"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

type MessagePage struct {
	Items      []PublicMessage `json:"items" binding:"required"`
	NextCursor *string         `json:"next_cursor,omitempty"`
	Through    string          `json:"through" binding:"required"`
}

type PublicMessage struct {
	MessageID      string `json:"message_id" binding:"required"`
	RunID          string `json:"run_id" binding:"required"`
	ConversationID int64  `json:"conversation_id" binding:"required"`
	Sequence       string `json:"sequence" binding:"required"`
	PartIndex      int64  `json:"part_index" binding:"required"`
	Kind           string `json:"kind" binding:"required"`
	Content        string `json:"content" binding:"required"`
}

type CurrentSubmission struct {
	LatestAccepted *AcceptedRun `json:"latest_accepted,omitempty"`
	ActiveRun      *struct {
		RootRunID        string  `json:"root_run_id" binding:"required"`
		SessionID        string  `json:"session_id" binding:"required"`
		AcceptedRevision int64   `json:"accepted_revision" binding:"required"`
		CreatedAt        string  `json:"created_at" binding:"required"`
		Status           string  `json:"status" binding:"required"`
		InputExcerpt     string  `json:"input_excerpt" binding:"required"`
		RetryOfRunID     *string `json:"retry_of_run_id,omitempty"`
	} `json:"active_run,omitempty"`
	UnresolvedRunIds *[]string `json:"unresolved_run_ids,omitempty"`
}

// OperationPaths is generated together with the request types.
var OperationPaths = map[string]string{
	"listAgentSessions":         "GET /internal/agent/sessions",
	"createAgentSession":        "POST /internal/agent/sessions",
	"getAgentSession":           "GET /internal/agent/sessions/{sid}",
	"updateAgentSession":        "PATCH /internal/agent/sessions/{sid}",
	"listAgentRuns":             "GET /internal/agent/sessions/{sid}/runs",
	"createAgentRun":            "POST /internal/agent/sessions/{sid}/runs",
	"getCurrentAgentSubmission": "GET /internal/agent/sessions/{sid}/submissions/current",
	"getAgentSubmission":        "GET /internal/agent/sessions/{sid}/submissions/{requestId}",
	"listAgentMessages":         "GET /internal/agent/runs/{rid}/messages",
	"getAgentMessage":           "GET /internal/agent/runs/{rid}/messages/{messageId}",
	"getAgentRun":               "GET /internal/agent/runs/{rid}",
	"applyAgentDecision":        "POST /internal/agent/runs/{rid}/commands",
	"subscribeAgentEvents":      "GET /internal/agent/runs/{rid}/events",
}
