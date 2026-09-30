package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func replayProjectCreation(tx *gorm.DB, actor identityapp.Principal, request application.ProjectCreationRequest, saved *domain.Project) (bool, error) {
	var locked int
	if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+request.Key.String()).Scan(&locked).Error; err != nil {
		return false, fmt.Errorf("lock project creation key: %w", err)
	}
	var row struct {
		RequestHash  string
		StatusCode   int
		ResponseBody []byte
	}
	result := tx.Raw(`SELECT request_hash,status_code,response_body FROM infra.idempotency_record
	 WHERE actor_id=? AND idem_key=? AND NOT is_delete AND expires_at>statement_timestamp()`, actor.ID, request.Key.String()).Scan(&row)
	if result.Error != nil {
		return false, fmt.Errorf("read project creation receipt: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if row.RequestHash != request.Hash || row.StatusCode != 201 {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal(row.ResponseBody, saved); err != nil {
		return false, fmt.Errorf("decode project creation receipt: %w", err)
	}
	if saved.ID == uuid.Nil || saved.OrgID != actor.OrgID {
		return false, application.ErrInvalidCreateProject
	}
	// Recheck visibility even for a replay. Later edits never replace the first
	// response, but a deleted or moved project cannot be disclosed from a receipt.
	var visible int
	result = tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete FOR SHARE`, saved.ID, actor.OrgID).Scan(&visible)
	if result.Error != nil {
		return false, fmt.Errorf("authorize project creation replay: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return false, ErrProjectNotFound
	}
	return true, nil
}

func recordProjectCreation(tx *gorm.DB, actor identityapp.Principal, request application.ProjectCreationRequest, saved domain.Project) error {
	body, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode project creation receipt: %w", err)
	}
	result := tx.Exec(`INSERT INTO infra.idempotency_record
	 (id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)
	 VALUES(?,?,?,?,201,?::jsonb,statement_timestamp()+interval '24 hours')
	 ON CONFLICT(actor_id,idem_key) DO UPDATE SET request_hash=excluded.request_hash,
	 status_code=excluded.status_code,response_body=excluded.response_body,
	 expires_at=excluded.expires_at,is_delete=false,update_time=statement_timestamp()`, uuid.New(), actor.ID, request.Key.String(), request.Hash, string(body))
	if result.Error != nil {
		return fmt.Errorf("record project creation receipt: %w", result.Error)
	}
	return nil
}
