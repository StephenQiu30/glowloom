package domain

type HumanGateReviewDecision struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID                    string
	SubjectType                                      string
	SubjectRevision                                  int
	SubjectHash, Decision                            string
	DecisionPayloadHash                              string
	ChangeRequest                                    *StructureIdentityChangeRequest
}

type HumanGateCoordination struct {
	ReviewDecisionID, DecisionStatus      string
	OwnerApplyStatus, OwnerReceiptID      string
	WorkflowResumeStatus, SignalReceiptID string
	ConflictCode                          string
	RepairWorkflowRunID                   string
}
