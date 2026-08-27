package domain

type HumanGateReviewDecision struct {
	WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID         string
	SubjectRevision                       int
	SubjectHash, Decision                 string
}

type HumanGateCoordination struct {
	ReviewDecisionID, DecisionStatus      string
	OwnerApplyStatus, OwnerReceiptID      string
	WorkflowResumeStatus, SignalReceiptID string
	ConflictCode                          string
}
