package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// Change persists one cancellation/retry command and its original result for replay.
func (s *ProjectCopyStore) Change(ctx context.Context, actor identityapp.Principal, id uuid.UUID, action string, expected int64, key uuid.UUID, requestID string) (domain.ProjectCopyJob, error) {
	if s == nil || s.db == nil || id == uuid.Nil || key == uuid.Nil || expected < 1 || (action != "retry" && action != "cancel" && action != "reconcile") {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	body, err := json.Marshal(struct {
		ID               uuid.UUID
		Action           string
		ExpectedRevision int64
	}{id, action, expected})
	if err != nil {
		return domain.ProjectCopyJob{}, err
	}
	digest := sha256.Sum256(append([]byte("project-copy-command/v1\x00"), body...))
	fingerprint := hex.EncodeToString(digest[:])
	var saved domain.ProjectCopyJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var locked int
		if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+key.String()).Scan(&locked).Error; err != nil {
			return err
		}
		before, err := readCopyJob(tx, actor, id, true)
		if err != nil {
			return err
		}
		var recorded struct {
			RequestSHA256 string
			ResponseBody  []byte
		}
		read := tx.Raw(`SELECT request_sha256,response_body FROM workspace.project_copy_command WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&recorded)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			if recorded.RequestSHA256 != fingerprint || copyJSON(recorded.ResponseBody, &saved) != nil || saved.Validate() != nil || saved.OrgID != actor.OrgID || saved.ID != id {
				return application.ErrIdempotencyConflict
			}
			return nil
		}
		var existing int
		if err := tx.Raw(`SELECT 1 FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>statement_timestamp()`, actor.ID, key.String()).Scan(&existing).Error; err != nil {
			return err
		}
		if existing == 1 {
			return application.ErrIdempotencyConflict
		}
		if before.Revision != expected {
			return domain.ErrProjectRevisionConflict
		}
		saved = before
		switch action {
		case "cancel":
			err = saved.RequestCancel()
		case "retry":
			err = saved.Retry()
		case "reconcile":
			err = saved.RequestReconciliation()
		}
		if err != nil {
			return err
		}
		if saved.Revision != before.Revision {
			if err := saveCopyJob(tx, before, saved); err != nil {
				return err
			}
			if err := copyChanged(tx, saved, action, time.Now()); err != nil {
				return err
			}
		}
		if err := copyAudit(tx, saved, actor, "project.copy_"+action, requestID, time.Now()); err != nil {
			return err
		}
		response, err := json.Marshal(saved)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO workspace.project_copy_command(id,copy_job_id,actor_id,idem_key,request_sha256,response_body) VALUES(?,?,?,?,?,?::jsonb)`, uuid.New(), saved.ID, actor.ID, key, fingerprint, string(response)).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?,?,?,?,202,?::jsonb,statement_timestamp()+interval '24 hours') ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,status_code=202,response_body=excluded.response_body,expires_at=excluded.expires_at,is_delete=false,update_time=statement_timestamp()`, uuid.New(), actor.ID, key.String(), fingerprint, string(response)).Error
	})
	return saved, err
}

// HasInflightWork protects both source pins and an unpublished target during lifecycle changes.
func (s *ProjectCopyStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil || actor.OrgID == uuid.Nil || projectID == uuid.Nil {
		return false, application.ErrProjectDependencyUnavailable
	}
	var present bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project_copy_job WHERE org_id=? AND (source_project_id=? OR target_project_id=?) AND status NOT IN ('succeeded','cancelled'))`, actor.OrgID, projectID, projectID).Scan(&present).Error
	return present, err
}

var _ application.ProjectCopyAdmissionStore = (*ProjectCopyStore)(nil)
var _ application.ProjectWorkGuard = (*ProjectCopyStore)(nil)
