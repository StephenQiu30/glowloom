package domain

import (
	"slices"
	"time"
)

type HumanTask struct {
	ID, WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	SubjectType, SubjectID                               string
	SubjectRevision                                      int
	CandidateIDs                                         []string
	RubricVersion                                        string
	Status                                               string
	ClaimedBy, ClaimToken                                *string
	ClaimExpiresAt                                       *time.Time
	Revision                                             int
	CreatedAt, UpdatedAt                                 time.Time
}

type ReviewDecision struct {
	ID, WorkspaceID, HumanTaskID string
	Decision                     string
	SubjectRevision              int
	SelectedCandidateID          *string
	CreatedBy                    string
	CreatedAt                    time.Time
}

type ClaimResult struct {
	Task       HumanTask
	ClaimToken string
}

type DecisionResult struct {
	Task     HumanTask
	Decision ReviewDecision
}

func SameTaskBinding(left, right HumanTask) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowRunID == right.WorkflowRunID && left.NodeRunID == right.NodeRunID &&
		left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID &&
		left.SubjectRevision == right.SubjectRevision && left.RubricVersion == right.RubricVersion &&
		slices.Equal(left.CandidateIDs, right.CandidateIDs)
}
