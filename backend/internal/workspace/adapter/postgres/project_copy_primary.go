package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// CopyPrimaryRepairStore is only composed by the explicit database operator CLI.
type CopyPrimaryRepairStore struct {
	db     *gorm.DB
	canvas func(*gorm.DB) application.CopyPrimaryInspector
}

// NewCopyPrimaryRepairStore binds the coordinator and canvas owner to one transaction.
func NewCopyPrimaryRepairStore(db *gorm.DB, canvas func(*gorm.DB) application.CopyPrimaryInspector) *CopyPrimaryRepairStore {
	return &CopyPrimaryRepairStore{db: db, canvas: canvas}
}

func (s *CopyPrimaryRepairStore) review(ctx context.Context, tx *gorm.DB, job domain.ProjectCopyJob) (application.CopyPrimaryReview, error) {
	invalid := application.ErrCopyPrimaryReviewChanged
	if job.Status != "succeeded" || job.Stage != "complete" || job.ExecutionUnconfirmed || job.NeedsReconciliation || job.CancellationRequested || job.CanvasReceipt == nil {
		return application.CopyPrimaryReview{}, invalid
	}
	frozen, err := readFrozenCopyWorkspace(tx, job)
	if err != nil {
		return application.CopyPrimaryReview{}, err
	}
	var project struct {
		Revision int64
		Status   string
	}
	read := tx.Raw(`SELECT revision,status FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete AND status IN ('active','archived') AND initial_canvas_id IS NULL`, job.TargetProjectID, job.OrgID).Scan(&project)
	if read.Error != nil {
		return application.CopyPrimaryReview{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.CopyPrimaryReview{}, invalid
	}
	inspector := s.canvas(tx)
	if inspector == nil {
		return application.CopyPrimaryReview{}, application.ErrProjectDependencyUnavailable
	}
	proof, err := inspector.InspectPrimary(ctx, copyCanvasBinding(job), copyCanvasSnapshot(job))
	if err != nil {
		return application.CopyPrimaryReview{}, err
	}
	receipt := job.CanvasReceipt
	if len(proof.Candidates) == 0 || receipt.ManifestSHA256 != proof.Receipt.ManifestSHA256 || receipt.ContentSHA256 != proof.Receipt.ContentSHA256 || receipt.PrimaryCount != proof.Receipt.Documents || receipt.SecondaryCount != proof.Receipt.ClearedOperationBindings {
		return application.CopyPrimaryReview{}, invalid
	}
	review := application.CopyPrimaryReview{OrgID: job.OrgID, JobID: job.ID, TargetProjectID: job.TargetProjectID, ProjectRevision: project.Revision, ProjectStatus: project.Status, WorkspaceHash: job.Manifest.WorkspaceSHA256, CanvasHash: job.Manifest.CanvasSHA256, FrozenPrimary: frozen.Target.CanvasID, Candidates: proof.Candidates}
	body, err := json.Marshal(review)
	if err != nil {
		return application.CopyPrimaryReview{}, err
	}
	hash := sha256.Sum256(body)
	review.ReviewHash = hex.EncodeToString(hash[:])
	return review, nil
}

// Review validates immutable evidence and inventories current candidates without writes.
func (s *CopyPrimaryRepairStore) Review(ctx context.Context, org, jobID uuid.UUID) (application.CopyPrimaryReview, error) {
	var review application.CopyPrimaryReview
	if s == nil || s.db == nil || s.canvas == nil || org == uuid.Nil || jobID == uuid.Nil {
		return review, application.ErrCopyPrimaryReviewChanged
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := readCopyJob(tx, identityapp.Principal{OrgID: org}, jobID, false)
		if err != nil {
			return err
		}
		review, err = s.review(ctx, tx, job)
		return err
	})
	return review, err
}

// Apply rechecks the review and atomically binds the primary with an audit receipt.
func (s *CopyPrimaryRepairStore) Apply(ctx context.Context, input application.CopyPrimaryRepair) (application.CopyPrimaryRepairReceipt, error) {
	var receipt application.CopyPrimaryRepairReceipt
	invalid := application.ErrCopyPrimaryReviewChanged
	hash, err := hex.DecodeString(input.ReviewHash)
	input.Reason = strings.TrimSpace(input.Reason)
	if s == nil || s.db == nil || s.canvas == nil || input.OrgID == uuid.Nil || input.JobID == uuid.Nil || input.SourceDocumentID == uuid.Nil || err != nil || len(hash) != 32 || hex.EncodeToString(hash) != input.ReviewHash || input.Reason == "" || utf8.RuneCountInString(input.Reason) > 500 {
		return receipt, invalid
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := readCopyJob(tx, identityapp.Principal{OrgID: input.OrgID}, input.JobID, true)
		if err != nil {
			return err
		}
		// The operator-only table is also the capability boundary: runtime roles
		// cannot get past this read, including by calling this adapter directly.
		previous := tx.Raw(`SELECT org_id,job_id,target_project_id,source_document_id,target_document_id,review_hash,before_revision,after_revision,reason,operator_name FROM workspace.project_copy_primary_repair WHERE job_id=?`, job.ID).Scan(&receipt)
		if previous.Error != nil {
			return previous.Error
		}
		if previous.RowsAffected == 1 {
			if receipt.OrgID != input.OrgID || receipt.SourceDocumentID != input.SourceDocumentID || receipt.ReviewHash != input.ReviewHash || receipt.Reason != input.Reason {
				return invalid
			}
			return nil // A replay returns the original historical receipt, not live state.
		}
		review, err := s.review(ctx, tx, job)
		if err != nil {
			return err
		}
		if review.ReviewHash != input.ReviewHash {
			return invalid
		}
		target := uuid.Nil
		for _, candidate := range review.Candidates {
			if candidate.SourceDocumentID == input.SourceDocumentID {
				target = candidate.TargetDocumentID
			}
		}
		if target == uuid.Nil || review.FrozenPrimary != uuid.Nil && review.FrozenPrimary != target {
			return invalid
		}
		updated := tx.Exec(`UPDATE workspace.project SET initial_canvas_id=?,revision=revision+1,update_time=clock_timestamp() WHERE id=? AND org_id=? AND revision=? AND initial_canvas_id IS NULL AND NOT is_delete AND status IN ('active','archived')`, target, job.TargetProjectID, job.OrgID, review.ProjectRevision)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return invalid
		}
		receipt = application.CopyPrimaryRepairReceipt{OrgID: job.OrgID, JobID: job.ID, TargetProjectID: job.TargetProjectID, SourceDocumentID: input.SourceDocumentID, TargetDocumentID: target, ReviewHash: input.ReviewHash, BeforeRevision: review.ProjectRevision, AfterRevision: review.ProjectRevision + 1, Reason: input.Reason}
		if err := tx.Raw(`SELECT session_user`).Scan(&receipt.OperatorName).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_primary_repair(job_id,org_id,target_project_id,source_document_id,target_document_id,review_hash,before_revision,after_revision,reason,operator_name) VALUES(?,?,?,?,?,?,?,?,?,?)`, receipt.JobID, receipt.OrgID, receipt.TargetProjectID, receipt.SourceDocumentID, receipt.TargetDocumentID, receipt.ReviewHash, receipt.BeforeRevision, receipt.AfterRevision, receipt.Reason, receipt.OperatorName).Error
	})
	if err != nil {
		return application.CopyPrimaryRepairReceipt{}, err
	}
	return receipt, nil
}
