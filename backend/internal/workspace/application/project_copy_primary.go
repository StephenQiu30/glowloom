package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
)

// ErrCopyPrimaryReviewChanged requires another explicit review before applying.
var ErrCopyPrimaryReviewChanged = errors.New("copy primary review changed or is ineligible")

// CopyPrimaryInspector is a maintenance-only, transaction-bound canvas owner port.
type CopyPrimaryInspector interface {
	InspectPrimary(context.Context, canvasapp.ProjectCopyBinding, canvasapp.ProjectCopySnapshot) (canvasapp.ProjectCopyPrimaryProof, error)
}

// CopyPrimaryReview is a read-only inventory; no candidate is chosen implicitly.
type CopyPrimaryReview struct {
	OrgID           uuid.UUID                               `json:"org_id"`
	JobID           uuid.UUID                               `json:"job_id"`
	TargetProjectID uuid.UUID                               `json:"target_project_id"`
	ProjectRevision int64                                   `json:"project_revision"`
	ProjectStatus   string                                  `json:"project_status"`
	WorkspaceHash   string                                  `json:"workspace_hash"`
	CanvasHash      string                                  `json:"canvas_hash"`
	FrozenPrimary   uuid.UUID                               `json:"frozen_primary"`
	Candidates      []canvasapp.ProjectCopyPrimaryCandidate `json:"candidates"`
	ReviewHash      string                                  `json:"review_hash"`
}

// CopyPrimaryRepair explicitly selects one reviewed frozen source document.
type CopyPrimaryRepair struct {
	OrgID, JobID, SourceDocumentID uuid.UUID
	ReviewHash, Reason             string
}

// CopyPrimaryRepairReceipt is persisted atomically with the navigation change.
type CopyPrimaryRepairReceipt struct {
	OrgID            uuid.UUID `json:"org_id"`
	JobID            uuid.UUID `json:"job_id"`
	TargetProjectID  uuid.UUID `json:"target_project_id"`
	SourceDocumentID uuid.UUID `json:"source_document_id"`
	TargetDocumentID uuid.UUID `json:"target_document_id"`
	ReviewHash       string    `json:"review_hash"`
	BeforeRevision   int64     `json:"before_revision"`
	AfterRevision    int64     `json:"after_revision"`
	Reason           string    `json:"reason"`
	OperatorName     string    `json:"operator_name"`
}
