package domain

import authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"

type ExecutionPlan struct {
	WorkflowRunID         string          `json:"workflow_run_id"`
	DefinitionVersionID   string          `json:"definition_version_id"`
	RunInputSnapshotID    string          `json:"run_input_snapshot_id"`
	DefinitionContentHash string          `json:"definition_content_hash"`
	InputSnapshotHash     string          `json:"input_snapshot_hash"`
	Nodes                 []ExecutionNode `json:"nodes"`
}

type ExecutionNode struct {
	NodeRunID string `json:"node_run_id"`
	NodeID    string `json:"node_id"`
	Executor  string `json:"executor"`
	RiskLevel string `json:"risk_level"`
}

type NodeActivityCommand struct {
	WorkflowRunID string `json:"workflow_run_id"`
	NodeRunID     string `json:"node_run_id"`
	NodeID        string `json:"node_id"`
	Executor      string `json:"executor"`
	Attempt       int    `json:"attempt"`
}

type NodeActivityResult struct {
	Status     string             `json:"status"`
	Output     NodeOutputSnapshot `json:"output"`
	OutputHash string             `json:"output_hash"`
}

type NodeExecutorResult struct {
	Status string             `json:"status"`
	Output NodeOutputSnapshot `json:"output"`
}

type NodeExecutionClaim struct {
	Command     NodeActivityCommand
	ClaimToken  string
	Status      string
	Attempt     int
	Revision    int
	Input       NodeInputSnapshot
	InputHash   string
	OutputPorts []authoring.PortDefinition
	Result      NodeActivityResult
	Replay      bool
}

type NodeExecutorCommand struct {
	NodeActivityCommand
	IdempotencyKey string
	Input          NodeInputSnapshot
	InputHash      string
	OutputPorts    []authoring.PortDefinition
}

type CompleteRunCommand struct {
	WorkflowRunID string `json:"workflow_run_id"`
}

type ApplyHumanGateCommand struct {
	WorkflowRunID  string `json:"workflow_run_id"`
	NodeRunID      string `json:"node_run_id"`
	NodeID         string `json:"node_id"`
	SignalIntentID string `json:"signal_intent_id"`
	Decision       string `json:"decision"`
}

type HumanGateBinding struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	SubjectType, SubjectID                           string
	SubjectRevision                                  int
	CandidateIDs                                     []string
	RubricVersion                                    string
}
