package postgres

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func copyAudit(tx *gorm.DB, job domain.ProjectCopyJob, actor identityapp.Principal, action, requestID string, now time.Time) error {
	request, err := uuid.Parse(requestID)
	if err != nil || request == uuid.Nil || request.String() != requestID {
		return domain.ErrInvalidProjectCopy
	}
	id := uuid.New()
	const topic = "lanverse.audit.recorded.v1"
	summary := map[string]any{"copy_job_id": job.ID, "source_project_id": job.SourceProjectID, "target_project_id": job.TargetProjectID, "status": job.Status, "stage": job.Stage, "revision": job.Revision, "documents": job.Manifest.Documents, "assets": job.Manifest.Assets, "renditions": job.Manifest.Renditions, "needs_reconciliation": job.NeedsReconciliation, "failure_code": job.FailureCode}
	payload, err := json.Marshal(map[string]any{"event_id": id, "event_type": topic, "occurred_at": now.UTC(), "org_id": job.OrgID, "project_id": job.SourceProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": id}, "data": map[string]any{"action": action, "object": map[string]any{"type": "project_copy", "id": job.ID}, "request_id": requestID, "before": nil, "after": summary}})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload)VALUES(?,?,?,?::jsonb)`, id, topic, job.SourceProjectID.String(), string(payload)).Error
}
