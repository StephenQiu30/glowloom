package domain

import (
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
)

const (
	NodeActivityNeedsAttention             = "NEEDS_ATTENTION"
	ProviderOutcomeUnknownErrorCode        = "provider_outcome_unknown"
	ManualProviderReconciliationNextAction = "manual_provider_reconciliation_required"
	VisionReviewUnconfirmedErrorCode       = "vision_review_unconfirmed"
	ManualVisionReviewNextAction           = "manual_vision_review_required"
)

func ValidNodeAttentionReason(code, nextAction string) bool {
	return (code == ProviderOutcomeUnknownErrorCode && nextAction == ManualProviderReconciliationNextAction) ||
		(code == VisionReviewUnconfirmedErrorCode && nextAction == ManualVisionReviewNextAction)
}

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
	ErrorCode  string             `json:"error_code,omitempty"`
	NextAction string             `json:"next_action,omitempty"`
}

type NodeExecutorResult struct {
	Status     string             `json:"status"`
	Output     NodeOutputSnapshot `json:"output"`
	ErrorCode  string             `json:"error_code,omitempty"`
	NextAction string             `json:"next_action,omitempty"`
}

type NodeExecutionClaim struct {
	Command                                 NodeActivityCommand
	ClaimToken                              string
	Status                                  string
	Attempt                                 int
	Revision                                int
	Input                                   NodeInputSnapshot
	InputHash                               string
	OutputPorts                             []authoring.PortDefinition
	WorkspaceID, ProjectID, InitiatorUserID string
	InitiatorTokenVersion                   int
	CachePolicy                             string
	CacheMaterial                           NodeCacheKeyMaterial
	CacheKey                                string
	Result                                  NodeActivityResult
	Replay                                  bool
}

type NodeExecutorCommand struct {
	NodeActivityCommand
	WorkspaceID, ProjectID, InitiatorUserID string
	InitiatorTokenVersion                   int
	IdempotencyKey                          string
	Input                                   NodeInputSnapshot
	InputHash                               string
	OutputPorts                             []authoring.PortDefinition
}

type StructureIdentityRepairDirective = agentcontract.StructureIdentityRepairDirective

type CompleteRunCommand struct {
	WorkflowRunID string `json:"workflow_run_id"`
}

type FailRunCommand struct {
	WorkflowRunID string `json:"workflow_run_id"`
	NodeRunID     string `json:"node_run_id"`
	NodeID        string `json:"node_id"`
	FailureCode   string `json:"failure_code"`
}

type ApplyHumanGateCommand struct {
	WorkflowRunID       string             `json:"workflow_run_id"`
	NodeRunID           string             `json:"node_run_id"`
	NodeID              string             `json:"node_id"`
	SignalIntentID      string             `json:"signal_intent_id"`
	Decision            string             `json:"decision"`
	DecisionPayloadHash string             `json:"decision_payload_hash"`
	OwnerReceiptID      string             `json:"owner_receipt_id"`
	Output              NodeOutputSnapshot `json:"output"`
	OutputHash          string             `json:"output_hash"`
}

type HumanGateBinding struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	Executor, InitiatorUserID                        string
	InitiatorTokenVersion                            int
	SubjectType, SubjectID                           string
	SubjectRevision                                  int
	SubjectHash                                      string
	CandidateIDs                                     []string
	CandidateSet                                     NodeInputBinding
	RubricVersion                                    string
	AllowedDecisions                                 []string
}
