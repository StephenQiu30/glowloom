package domain

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
	Status string `json:"status"`
}

type NodeExecutionClaim struct {
	Command    NodeActivityCommand
	ClaimToken string
	Status     string
	Attempt    int
	Revision   int
	Replay     bool
}

type NodeExecutorCommand struct {
	NodeActivityCommand
	IdempotencyKey string
}

type CompleteRunCommand struct {
	WorkflowRunID string `json:"workflow_run_id"`
}

type HumanGateBinding struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	SubjectType, SubjectID                           string
	SubjectRevision                                  int
	CandidateIDs                                     []string
	RubricVersion                                    string
}
