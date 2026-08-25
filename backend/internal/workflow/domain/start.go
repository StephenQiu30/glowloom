package domain

import (
	"encoding/json"
	"time"
)

const (
	StartOutcomeStarted        = "started"
	StartOutcomeAlreadyStarted = "already_started"
	StartOutcomeUnknown        = "unknown"
	StartOutcomeConflict       = "conflict"
)

type StartRequest struct {
	WorkflowID            string `json:"workflow_id"`
	WorkflowType          string `json:"workflow_type"`
	WorkflowTypeVersion   string `json:"workflow_type_version"`
	WorkflowRunID         string `json:"workflow_run_id"`
	DefinitionVersionID   string `json:"definition_version_id"`
	RunInputSnapshotID    string `json:"run_input_snapshot_id"`
	DefinitionContentHash string `json:"definition_content_hash"`
	InputSnapshotHash     string `json:"input_snapshot_hash"`
	InputHash             string `json:"input_hash"`
}

type StartObservation struct {
	Outcome           string `json:"outcome"`
	ObservedInputHash string `json:"observed_input_hash,omitempty"`
}

type WorkflowRun struct {
	ID, WorkspaceID, ProjectID, AuthoringRevisionID string
	DefinitionVersionID, RunInputSnapshotID         string
	TemporalWorkflowID, StartInputHash              string
	Status, ProgressStage                           string
	NextAction                                      *string
	Error                                           json.RawMessage
	Revision                                        int
	CreatedBy                                       string
	CreatedAt, UpdatedAt                            time.Time
}

type NodeRunProjection struct {
	ID, WorkspaceID, WorkflowRunID string
	NodeID, DefinitionKey          string
	DefinitionVersion              string
	Status                         string
	Attempt, Revision              int
	CreatedAt, UpdatedAt           time.Time
}

type StartIntent struct {
	ID, WorkspaceID, WorkflowRunID string
	IdempotencyKey                 string
	CommandInputHash               string
	TemporalInputHash              string
	Status                         string
	AttemptNo, Revision            int
	CreatedBy                      string
	CreatedAt, UpdatedAt           time.Time
}

type StartReceipt struct {
	ID, WorkspaceID, StartIntentID, WorkflowRunID string
	AttemptNo                                     int
	Outcome, TemporalWorkflowID                   string
	ExpectedInputHash                             string
	ObservedInputHash                             *string
	CreatedAt                                     time.Time
}

type StartPreparation struct {
	Run    WorkflowRun
	Nodes  []NodeRunProjection
	Intent StartIntent
}
