package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func replayProjectCreation(tx *gorm.DB, actor identityapp.Principal, request application.ProjectCreationRequest, saved *domain.Project) (bool, error) {
	var locked int
	if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+request.Key.String()).Scan(&locked).Error; err != nil {
		return false, fmt.Errorf("lock project creation key: %w", err)
	}
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	result := tx.Raw(`SELECT request_hash,response_body FROM workspace.project_creation_receipt
	 WHERE actor_id=? AND idem_key=?`, actor.ID, request.Key.String()).Scan(&row)
	if result.Error != nil {
		return false, fmt.Errorf("read project creation receipt: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if row.RequestHash != request.Hash {
		return false, application.ErrIdempotencyConflict
	}
	if err := json.Unmarshal(row.ResponseBody, saved); err != nil {
		return false, fmt.Errorf("decode project creation receipt: %w", err)
	}
	if saved.ID == uuid.Nil || saved.OrgID != actor.OrgID || saved.OwnerID != actor.ID || saved.CanvasID == uuid.Nil {
		return false, application.ErrInvalidCreateProject
	}
	// Recheck visibility even for a replay. Later edits never replace the first
	// response, but a deleted or moved project cannot be disclosed from a receipt.
	var visible int
	result = tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? AND NOT is_delete FOR SHARE`, saved.ID, actor.OrgID, actor.ScopeID).Scan(&visible)
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
	result := tx.Exec(`INSERT INTO workspace.project_creation_receipt
     (actor_id,idem_key,request_hash,project_id,response_body) VALUES(?,?,?,?,?::jsonb)`, actor.ID, request.Key, request.Hash, saved.ID, string(body))
	if result.Error != nil {
		return fmt.Errorf("record project creation receipt: %w", result.Error)
	}
	return nil
}
