package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

const workflowControlTopic = "lanverse.workflow.control_requested.v1"
const controlAuditTopic = "lanverse.audit.recorded.v1"

// RequestWorkflowControl atomically persists one authorized intent and its receipt.
// Lifecycle and settlement remain owned by the workflow after signal delivery.
func (s *Store) RequestWorkflowControl(ctx context.Context, actor identityapp.Principal, input application.WorkflowControlInput) (application.WorkflowControlResult, error) {
	var result application.WorkflowControlResult
	if s == nil || s.db == nil {
		return result, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return result, err
	}
	requestID := uuid.MustParse(input.RequestID)
	sum := sha256.Sum256([]byte("workflow-control:" + actor.OrgID.String() + ":" + input.ProjectID.String() + ":" + input.TargetType + ":" + input.TargetID.String() + ":" + input.Action))
	hash := hex.EncodeToString(sum[:])
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePublicProject(tx, actor, input.ProjectID, true); err != nil {
			return err
		}
		if err := lockQuoteRequest(tx, actor, requestID); err != nil {
			return err
		}
		var previous struct {
			RequestHash  string
			ResponseBody []byte
		}
		read := tx.Raw(`SELECT request_hash,response_body FROM infra.idempotency_record WHERE actor_id=?::uuid AND idem_key=? AND NOT is_delete`, actor.ID, input.RequestID).Scan(&previous)
		if read.Error != nil {
			return fmt.Errorf("read workflow control receipt: %w", read.Error)
		}
		if read.RowsAffected == 1 {
			if previous.RequestHash != hash {
				return application.ErrWorkflowControlKeyReused
			}
			if err := json.Unmarshal(previous.ResponseBody, &result); err != nil {
				return fmt.Errorf("decode workflow control receipt: %w", err)
			}
			return nil
		}
		status, err := lockControlTarget(tx, input)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		eventID := uuid.New()
		result = application.WorkflowControlResult{RequestID: requestID, EventID: eventID, TargetID: input.TargetID, TargetType: input.TargetType, Action: input.Action, Accepted: true}
		payload, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": workflowControlTopic, "occurred_at": now, "org_id": actor.OrgID, "project_id": input.ProjectID,
			"actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": input.TargetType, "id": input.TargetID}, "data": map[string]any{"action": input.Action, "request_id": requestID}})
		if err != nil {
			return fmt.Errorf("encode workflow control: %w", err)
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?::uuid,?,?,?::jsonb)`, eventID, workflowControlTopic, input.ProjectID.String(), string(payload)).Error; err != nil {
			return fmt.Errorf("persist workflow control: %w", err)
		}
		if input.TargetType == "operation" {
			if err := tx.Exec(`INSERT INTO operation.operation_event(id,operation_id,from_status,to_status,reason,detail) VALUES(?::uuid,?::uuid,?,?,'user_cancel_requested','{}'::jsonb)`, uuid.New(), input.TargetID, status, status).Error; err != nil {
				return fmt.Errorf("record task cancel intent: %w", err)
			}
		}
		auditID := uuid.New()
		action := input.TargetType + "." + input.Action + "_requested"
		audit, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": controlAuditTopic, "occurred_at": now, "org_id": actor.OrgID, "project_id": input.ProjectID,
			"actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": action, "object": map[string]any{"type": input.TargetType, "id": input.TargetID}, "request_id": input.RequestID, "after": map[string]any{"status": status}}})
		if err != nil {
			return fmt.Errorf("encode control audit: %w", err)
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?::uuid,?,?,?::jsonb)`, auditID, controlAuditTopic, input.ProjectID.String(), string(audit)).Error; err != nil {
			return fmt.Errorf("persist control audit: %w", err)
		}
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at) VALUES(?::uuid,?::uuid,?,?,202,?::jsonb,now()+interval '24 hours')`, uuid.New(), actor.ID, input.RequestID, hash, string(body)).Error; err != nil {
			return fmt.Errorf("persist workflow control receipt: %w", err)
		}
		return nil
	})
	return result, err
}

func lockControlTarget(tx *gorm.DB, input application.WorkflowControlInput) (string, error) {
	var row struct {
		Status            string
		PausedReason      *string
		CancelRequestedAt *time.Time
		SupportsCancel    bool
	}
	var read *gorm.DB
	if input.TargetType == "operation" {
		read = tx.Raw(`SELECT o.status,COALESCE(v.supports_cancel,false) AS supports_cancel FROM operation.operation AS o
 LEFT JOIN catalog.model_profile_version AS v ON v.id=o.model_profile_version_id
 LEFT JOIN catalog.model_profile AS m ON m.id=v.model_profile_id
 LEFT JOIN catalog.provider AS p ON p.id=m.provider_id
 WHERE o.id=?::uuid AND o.project_id=?::uuid AND NOT o.is_delete FOR UPDATE OF o`, input.TargetID, input.ProjectID).Scan(&row)
	} else {
		read = tx.Raw(`SELECT status,paused_reason,cancel_requested_at FROM operation.batch WHERE id=?::uuid AND project_id=?::uuid AND NOT is_delete FOR UPDATE`, input.TargetID, input.ProjectID).Scan(&row)
	}
	if read.Error != nil {
		return "", fmt.Errorf("lock workflow control target: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return "", application.ErrPublicNotFound
	}
	switch {
	case input.TargetType == "operation":
		if row.Status != string(domain.StatusConfirmed) && (row.Status != string(domain.StatusSubmitted) || !row.SupportsCancel) {
			return "", application.ErrWorkflowControlConflict
		}
	case input.Action == "cancel":
		if row.Status != string(domain.BatchStatusConfirmed) && row.Status != string(domain.BatchStatusRunning) {
			return "", application.ErrWorkflowControlConflict
		}
	case row.Status != string(domain.BatchStatusRunning) || row.PausedReason == nil || row.CancelRequestedAt != nil:
		return "", application.ErrWorkflowControlConflict
	}
	return row.Status, nil
}

var _ application.WorkflowControlStore = (*Store)(nil)
