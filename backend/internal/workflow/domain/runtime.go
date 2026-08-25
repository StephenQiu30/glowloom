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
