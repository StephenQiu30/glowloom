package domain

import (
	"encoding/json"
	"slices"
	"time"
)

type HumanTask struct {
	ID, WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	SubjectType, SubjectID                               string
	SubjectRevision                                      int
	SubjectHash                                          string
	CandidateIDs                                         []string
	RubricVersion                                        string
	AllowedDecisions                                     []string
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
	SubjectHash                  string
	SelectedCandidateID          *string
	ChangeRequest                *ChangeRequest
	DecisionPayloadHash          string
	CreatedBy                    string
	CreatedAt                    time.Time
}

type ChangeEvidenceRef struct {
	SourceVersionID string `json:"source_version_id"`
	SourceStart     int    `json:"source_start"`
	SourceEnd       int    `json:"source_end"`
	TextHash        string `json:"text_hash"`
}

type ChangeSpec struct {
	Operation         string   `json:"operation"`
	TargetKeys        []string `json:"target_keys"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
}

type ChangeRequest struct {
	IssueRefs    []string            `json:"issue_refs"`
	EvidenceRefs []ChangeEvidenceRef `json:"evidence_refs"`
	ChangeSpec   ChangeSpec          `json:"change_spec"`
	ReasonCode   string              `json:"reason_code"`
	UserNote     *string             `json:"user_note,omitempty"`
}

type ClaimResult struct {
	Task       HumanTask
	ClaimToken string
}

type DecisionResult struct {
	Task     HumanTask
	Decision ReviewDecision
}

type HumanTaskPage struct {
	Tasks     []HumanTask
	NextAfter *string
}

type HumanTaskDetail struct {
	Task     HumanTask
	Decision *ReviewDecision
	Subject  json.RawMessage
}

func SameTaskBinding(left, right HumanTask) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.WorkflowRunID == right.WorkflowRunID && left.NodeRunID == right.NodeRunID &&
		left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID &&
		left.SubjectRevision == right.SubjectRevision && left.SubjectHash == right.SubjectHash &&
		left.RubricVersion == right.RubricVersion && slices.Equal(left.CandidateIDs, right.CandidateIDs) &&
		slices.Equal(left.AllowedDecisions, right.AllowedDecisions)
}
