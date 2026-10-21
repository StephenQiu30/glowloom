package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectRetentionContent removes only module-owned, expired project content.
// Implementations use the transaction that holds the project/deletion batch lock.
type ProjectRetentionContent interface {
	ClearProjectContent(context.Context, uuid.UUID) error
}

// ProjectRetentionStore owns bounded, durable cleanup of expired deletion batches.
// It is an internal worker store, never a user-facing delete endpoint.
type ProjectRetentionStore struct {
	db      *gorm.DB
	content func(*gorm.DB) ProjectRetentionContent
}

// NewProjectRetentionStore injects transaction-bound content owners.
func NewProjectRetentionStore(db *gorm.DB, content func(*gorm.DB) ProjectRetentionContent) *ProjectRetentionStore {
	return &ProjectRetentionStore{db: db, content: content}
}

// EnqueueExpired records one immutable job for each expired project revision.
func (s *ProjectRetentionStore) EnqueueExpired(ctx context.Context, limit int) (int64, error) {
	if s == nil || s.db == nil || limit < 1 || limit > 500 {
		return 0, ErrUnavailable
	}
	result := s.db.WithContext(ctx).Exec(`INSERT INTO workspace.project_retention_job(project_id,org_id,owner_scope_id,deleted_revision,purge_after)
 SELECT p.id,p.org_id,p.owner_scope_id,p.revision,p.purge_after FROM workspace.project p
 WHERE p.is_delete AND p.status IN ('active','archived') AND p.owner_scope_id IS NOT NULL
 AND p.delete_time IS NOT NULL AND p.purge_after>p.delete_time AND p.purge_after<=clock_timestamp()
 AND NOT EXISTS(SELECT 1 FROM workspace.project_retention_job j WHERE j.project_id=p.id AND j.deleted_revision=p.revision)
 ORDER BY p.purge_after,p.id LIMIT ? ON CONFLICT(project_id,deleted_revision) DO NOTHING`, limit)
	if result.Error != nil {
		return 0, fmt.Errorf("enqueue expired projects: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ProcessNext locks a job and its exact project deletion version until cleanup commits.
// A rollback keeps the job pending; retries cannot touch a restored/newer version.
func (s *ProjectRetentionStore) ProcessNext(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil || s.content == nil {
		return false, ErrUnavailable
	}
	var job struct {
		ProjectID, OrgID, OwnerScopeID uuid.UUID
		DeletedRevision                int64
		PurgeAfter                     time.Time
	}
	found := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := tx.Raw(`SELECT project_id,org_id,owner_scope_id,deleted_revision,purge_after FROM workspace.project_retention_job WHERE status='pending' ORDER BY purge_after,project_id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&job)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		found = true
		var p struct {
			OrgID, OwnerScopeID uuid.UUID
			Revision            int64
			IsDelete            bool
			PurgeAfter          *time.Time
			DeleteTime          *time.Time
		}
		r = tx.Raw(`SELECT org_id,owner_scope_id,revision,is_delete,purge_after,delete_time FROM workspace.project WHERE id=? FOR UPDATE`, job.ProjectID).Scan(&p)
		if r.Error != nil {
			return r.Error
		}
		var now time.Time
		if err := tx.Raw(`SELECT clock_timestamp()`).Scan(&now).Error; err != nil {
			return err
		}
		status := "skipped"
		if r.RowsAffected == 1 && p.IsDelete && p.Revision == job.DeletedRevision && p.OrgID == job.OrgID && p.OwnerScopeID == job.OwnerScopeID && p.DeleteTime != nil && p.PurgeAfter != nil && p.PurgeAfter.Equal(job.PurgeAfter) && p.PurgeAfter.After(*p.DeleteTime) && !now.Before(*p.PurgeAfter) {
			owner := s.content(tx)
			if owner == nil {
				return ErrUnavailable
			}
			if err := owner.ClearProjectContent(ctx, job.ProjectID); err != nil {
				return fmt.Errorf("clear expired project content: %w", err)
			}
			status = "completed"
		}
		return tx.Exec(`UPDATE workspace.project_retention_job SET status=?,completed_at=?,attempts=attempts+1,last_error=NULL WHERE project_id=? AND deleted_revision=?`, status, now, job.ProjectID, job.DeletedRevision).Error
	})
	if err != nil && found {
		// Record only an error category; SQL errors may include private payloads.
		recordErr := s.db.WithContext(ctx).Exec(`UPDATE workspace.project_retention_job SET attempts=attempts+1,last_error='content_cleanup_failed' WHERE project_id=? AND deleted_revision=? AND status='pending'`, job.ProjectID, job.DeletedRevision).Error
		if recordErr != nil {
			return false, fmt.Errorf("record project retention failure: %w (cleanup: %w)", recordErr, err)
		}
	}
	return found, err
}
