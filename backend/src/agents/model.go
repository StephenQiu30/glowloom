package agents

import (
	"time"

	"github.com/google/uuid"
)

type AgentRun struct {
	ID                uuid.UUID `json:"id"`
	ProjectID         uuid.UUID `json:"project_id"`
	OperationID       uuid.UUID `json:"operation_id"`
	Skill             string    `json:"skill"`
	Stage             string    `json:"stage"`
	StageGeneration   int       `json:"stage_generation"`
	RequestHash       string    `json:"request_hash"`
	Status            string    `json:"status"`
	InputSnapshotHash string    `json:"input_snapshot_hash"`
	ResultHash        string    `json:"result_hash,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ProposalItem struct {
	ID            uuid.UUID `json:"id"`
	AgentRunID    uuid.UUID `json:"agent_run_id"`
	TargetModule  string    `json:"target_module"`
	TargetCommand string    `json:"target_command"`
	Payload       any       `json:"payload"`
	Decision      string    `json:"decision"`
	ReadSetHash   string    `json:"read_set_hash"`
	WriteSetHash  string    `json:"write_set_hash"`
}
