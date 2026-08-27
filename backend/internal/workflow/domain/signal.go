package domain

import (
	"encoding/json"
	"strconv"
	"time"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
)

const (
	SignalOutcomeSignaled       = "signaled"
	SignalOutcomeAlreadyApplied = "already_applied"
	SignalOutcomeUnknown        = "unknown"
	SignalOutcomeConflict       = "conflict"
)

type HumanGateApplyReceipt struct {
	ID, WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID             string
	SubjectRevision                           int
	Decision, Status, ConflictCode            string
	OwnerReceiptID, OwnerOperation            string
	Output                                    NodeOutputSnapshot
	OutputHash                                string
	CreatedBy                                 string
	CreatedAt                                 time.Time
}

type HumanGateDecisionRequest struct {
	WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID         string
	SubjectRevision                       int
	Decision                              string
}

type HumanGateOwnerApplication struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID                    string
	SubjectRevision                                  int
	Decision, Executor                               string
	Candidate                                        NodeInputBinding
	OutputPort, OutputValueType                      string
	NodeConfig                                       json.RawMessage
	FrozenInputs                                     []authoring.FrozenReference
}

type HumanGateOwnerResult struct {
	ReceiptID, Operation string
	Output               NodeOutputSnapshot
	OutputHash           string
}

type SignalIntent struct {
	ID, WorkspaceID, WorkflowRunID, NodeRunID string
	HumanTaskID, ReviewDecisionID             string
	IdempotencyKey, CommandInputHash          string
	TemporalWorkflowID, SignalID, InputHash   string
	Decision                                  string
	SubjectRevision                           int
	Status                                    string
	AttemptNo, Revision                       int
	CreatedBy                                 string
	CreatedAt, UpdatedAt                      time.Time
}

type SignalReceipt struct {
	ID, WorkspaceID, SignalIntentID, WorkflowRunID string
	AttemptNo                                      int
	Outcome, SignalID                              string
	ExpectedInputHash                              string
	ObservedInputHash                              *string
	CreatedAt                                      time.Time
}

type SignalPreparation struct {
	ApplyReceipt HumanGateApplyReceipt
	Intent       SignalIntent
}

type SignalRequest struct {
	TemporalWorkflowID string             `json:"temporal_workflow_id"`
	SignalID           string             `json:"signal_id"`
	SignalIntentID     string             `json:"signal_intent_id"`
	WorkflowRunID      string             `json:"workflow_run_id"`
	NodeRunID          string             `json:"node_run_id"`
	Decision           string             `json:"decision"`
	OwnerReceiptID     string             `json:"owner_receipt_id"`
	Output             NodeOutputSnapshot `json:"output"`
	OutputHash         string             `json:"output_hash"`
	InputHash          string             `json:"input_hash"`
}

type SignalObservation struct {
	Outcome           string
	ObservedInputHash string
}

func HumanGateOutputMatchesCandidate(
	executor string,
	candidate NodeInputBinding,
	output NodeOutputBinding,
) bool {
	if executor == "gate.production_bible_review" {
		candidateRevision, candidateErr := strconv.ParseInt(candidate.ReferenceVersion, 10, 64)
		version, versionErr := strconv.Atoi(output.ReferenceVersion)
		return candidate.ValueType == "story_reconciliation_candidate" && candidateErr == nil && candidateRevision >= 1 &&
			output.ValueType == "production_bible_version" && versionErr == nil && version >= 1 &&
			output.ReferenceID != candidate.ReferenceID && len(candidate.ContentHash) == 64 && len(output.ContentHash) == 64
	}
	if executor == "gate.generation_image_review" {
		return candidate.ValueType == "generation_candidate_set" && candidate.ReferenceVersion == "1" &&
			output.ValueType == "generation_candidate_selection" && output.ReferenceVersion == "1" &&
			output.ReferenceID != candidate.ReferenceID
	}
	if output.ReferenceID != candidate.ReferenceID {
		return false
	}
	if executor != "gate.storyboard_review" {
		return output.ContentHash == candidate.ContentHash
	}
	candidateRevision, err := strconv.Atoi(candidate.ReferenceVersion)
	return err == nil && candidateRevision >= 1 &&
		output.ReferenceVersion == strconv.Itoa(candidateRevision+1) &&
		output.ContentHash != candidate.ContentHash
}
