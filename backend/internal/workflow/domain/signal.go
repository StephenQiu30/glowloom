package domain

import "time"

const (
	SignalOutcomeSignaled       = "signaled"
	SignalOutcomeAlreadyApplied = "already_applied"
	SignalOutcomeUnknown        = "unknown"
	SignalOutcomeConflict       = "conflict"
)

type HumanGateApplyReceipt struct {
	ID, WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID             string
	SubjectRevision                           int
	Decision                                  string
	CreatedBy                                 string
	CreatedAt                                 time.Time
}

type SignalIntent struct {
	ID, WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID             string
	IdempotencyKey, CommandInputHash          string
	TemporalWorkflowID, SignalID, InputHash   string
	Decision                                  string
	SubjectRevision                           int
	Status                                    string
	AttemptNo, Revision                       int
	CreatedBy                                 string
	CreatedAt, UpdatedAt                      time.Time
}

type SignalReceipt struct {
	ID, WorkspaceID, SignalIntentID, WorkflowRunID string
	AttemptNo                                      int
	Outcome, SignalID                              string
	ExpectedInputHash                              string
	ObservedInputHash                              *string
	CreatedAt                                      time.Time
}

type SignalPreparation struct {
	ApplyReceipt HumanGateApplyReceipt
	Intent       SignalIntent
}

type SignalRequest struct {
	TemporalWorkflowID string `json:"temporal_workflow_id"`
	SignalID           string `json:"signal_id"`
	SignalIntentID     string `json:"signal_intent_id"`
	WorkflowRunID      string `json:"workflow_run_id"`
	NodeRunID          string `json:"node_run_id"`
	Decision           string `json:"decision"`
	InputHash          string `json:"input_hash"`
}

type SignalObservation struct {
	Outcome           string
	ObservedInputHash string
}
