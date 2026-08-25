package domain

import "time"

const (
	ControlActionCancel = "cancel"

	ControlOutcomeRequested      = "requested"
	ControlOutcomeApplied        = "applied"
	ControlOutcomeAlreadyApplied = "already_applied"
	ControlOutcomeUnknown        = "unknown"
	ControlOutcomeConflict       = "conflict"
)

type ControlRequest struct {
	TemporalWorkflowID string `json:"temporal_workflow_id"`
	ControlID          string `json:"control_id"`
	WorkflowRunID      string `json:"workflow_run_id"`
	Action             string `json:"action"`
	InputHash          string `json:"input_hash"`
}

type ControlObservation struct {
	Outcome           string `json:"outcome"`
	ObservedInputHash string `json:"observed_input_hash,omitempty"`
}

type ControlIntent struct {
	ID, WorkspaceID, WorkflowRunID   string
	IdempotencyKey, CommandInputHash string
	TemporalWorkflowID, ControlID    string
	InputHash, Action                string
	ExpectedRunRevision              int
	Status                           string
	AttemptNo, Revision              int
	CreatedBy                        string
	CreatedAt, UpdatedAt             time.Time
}

type ControlReceipt struct {
	ID, WorkspaceID, ControlIntentID, WorkflowRunID string
	AttemptNo                                       int
	Outcome, ControlID, ExpectedInputHash           string
	ObservedInputHash                               *string
	CreatedAt                                       time.Time
}

type ControlPreparation struct {
	Run    WorkflowRun
	Intent ControlIntent
}

type ControlFinalization struct {
	Run                       WorkflowRun
	Intent                    ControlIntent
	Receipt                   ControlReceipt
	ExpectedRunRevision       int
	ExpectedIntentRevision    int
	CancelNonTerminalNodeRuns bool
}
