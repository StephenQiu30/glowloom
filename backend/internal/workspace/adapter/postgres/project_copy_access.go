package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// ProjectCopyAccessStore retains the coordinator's locks in the owner's transaction.
type ProjectCopyAccessStore struct {
	tx        *gorm.DB
	requests  RequestScopeFactory
	workers   WorkerScopeFactory
	authority application.ProjectCopyAuthority
}

// NewProjectCopyAccessStore binds each own transaction to one trusted admission/attempt.
func NewProjectCopyAccessStore(tx *gorm.DB, authority application.ProjectCopyAuthority) *ProjectCopyAccessStore {
	return &ProjectCopyAccessStore{tx: tx, authority: authority}
}

// Authorize requires current workspace authority for the exact private copy scope.
func (s *ProjectCopyAccessStore) Authorize(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, _ bool) error {
	if s == nil || s.tx == nil || s.tx.Statement == nil {
		return application.ErrProjectDependencyUnavailable
	}
	if _, transaction := s.tx.Statement.ConnPool.(gorm.TxCommitter); !transaction {
		return application.ErrProjectDependencyUnavailable
	}
	a := s.authority
	if binding != a.Binding || binding.JobID == uuid.Nil || binding.OrgID != actor.OrgID || a.ActorID != actor.ID || binding.SourceProjectID == uuid.Nil || binding.TargetProjectID == uuid.Nil || binding.SourceProjectID == binding.TargetProjectID || a.SourceRevision < 1 {
		return domain.ErrInvalidProjectCopy
	}
	tx := s.tx.WithContext(ctx)
	scope := actor.ScopeID
	if a.Phase == "freeze" {
		if err := (&Store{scopes: s.requests}).requireScope(ctx, tx, actor, true); err != nil {
			return err
		}
	} else {
		copyStore := &ProjectCopyStore{requests: s.requests, workers: s.workers}
		if _, err := copyStore.authorizedCopy(ctx, tx, actor, binding.JobID, true, true); err != nil {
			return err
		}
		var row struct{ OwnerScopeID uuid.UUID }
		if err := tx.Raw(`SELECT owner_scope_id FROM workspace.project_copy_job WHERE id=?`, binding.JobID).Scan(&row).Error; err != nil {
			return err
		}
		scope = row.OwnerScopeID
	}
	var projects []struct {
		ID       uuid.UUID
		Revision int64
		Status   string
		IsDelete bool
	}
	read := tx.Raw(`SELECT id,revision,status,is_delete FROM workspace.project WHERE org_id=? AND owner_scope_id=? AND id IN (?,?) ORDER BY id FOR UPDATE`, binding.OrgID, scope, binding.SourceProjectID, binding.TargetProjectID).Scan(&projects)
	if read.Error != nil {
		return fmt.Errorf("lock copying project scope: %w", read.Error)
	}
	if len(projects) != 2 {
		return application.ErrProjectNotFound
	}
	for _, project := range projects {
		if project.IsDelete || project.ID == binding.TargetProjectID && project.Status != "copying" || project.ID == binding.SourceProjectID && project.Status != "active" && project.Status != "archived" {
			return domain.ErrProjectCopyStateConflict
		}
		if a.Phase == "freeze" && project.ID == binding.SourceProjectID && project.Revision != a.SourceRevision {
			return domain.ErrProjectRevisionConflict
		}
	}
	if a.Phase == "freeze" {
		if a.WorkerID != uuid.Nil {
			return domain.ErrProjectCopyWorkerConflict
		}
		return nil
	}
	if a.WorkerID == uuid.Nil {
		return domain.ErrProjectCopyWorkerConflict
	}
	job, err := readCopyJob(tx, actor, binding.JobID, true)
	if err != nil {
		return err
	}
	if job.OrgID != binding.OrgID || job.ExecutionActor() != a.ActorID || job.SourceProjectID != binding.SourceProjectID || job.TargetProjectID != binding.TargetProjectID || job.SourceRevision != a.SourceRevision {
		return domain.ErrInvalidProjectCopy
	}
	if job.WorkerID != a.WorkerID {
		return domain.ErrProjectCopyWorkerConflict
	}
	switch a.Phase {
	case "bible_transfer":
		if job.Status == "running" && !job.CancellationRequested && job.Stage == "bible" && job.Manifest.Bible != nil {
			return nil
		}
	case "bible_register":
		if job.Status == "running" && !job.CancellationRequested && (job.Stage == "bible" || job.Stage == "finalizing") && job.Manifest.Bible != nil {
			return nil
		}
	case "bible_verify":
		if job.Status == "running" && !job.CancellationRequested && job.Stage == "finalizing" && job.Manifest.Bible != nil && job.BibleReceipt != nil {
			return nil
		}
	case "transfer":
		if job.Status == "running" && !job.CancellationRequested && job.Stage == "script" {
			return nil
		}
	case "register":
		if job.Status == "running" && !job.CancellationRequested && (job.Stage == "script" || job.Stage == "finalizing") {
			return nil
		}
	case "cleanup":
		if job.Status == "cancel_requested" && job.CancellationRequested && job.Stage == "cleanup" {
			return nil
		}
	default:
		return domain.ErrInvalidProjectCopy
	}
	return domain.ErrProjectCopyStateConflict
}
