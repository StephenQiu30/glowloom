package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// ReadCreationRecovery reads only committed receipts; an absent receipt never proves no POST is in flight.
func (s *Store) ReadCreationRecovery(ctx context.Context, actor identity.Principal, key uuid.UUID) (application.ProjectCreationRecovery, error) {
	result := application.ProjectCreationRecovery{State: "unrecorded"}
	if s == nil || s.db == nil {
		return result, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireScope(ctx, tx, actor, false); err != nil {
			return err
		}
		var receipt struct {
			ProjectID    uuid.UUID
			ResponseBody []byte
			RequestHash  string
		}
		read := tx.Raw(`SELECT project_id,response_body,request_hash FROM workspace.project_creation_receipt WHERE actor_id=? AND idem_key=?`, actor.ID, key.String()).Scan(&receipt)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 0 {
			return nil
		}
		var original domain.Project
		if err := json.Unmarshal(receipt.ResponseBody, &original); err != nil {
			return fmt.Errorf("decode creation recovery: %w", err)
		}
		if original.ID == uuid.Nil || original.ID != receipt.ProjectID || original.CanvasID == uuid.Nil || original.OwnerID != actor.ID || len(receipt.RequestHash) != 64 {
			return application.ErrProjectDependencyUnavailable
		}
		if original.OrgID != actor.OrgID {
			return application.ErrProjectNotFound
		}
		var current struct {
			ID, InitialCanvasID uuid.UUID
			Status              string
			IsDelete            bool
		}
		read = tx.Raw(`SELECT id,initial_canvas_id,status,is_delete FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? FOR SHARE`, original.ID, actor.OrgID, actor.ScopeID).Scan(&current)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrProjectNotFound
		}
		if current.IsDelete {
			return application.ErrCreationTargetUnavailable
		}
		if current.InitialCanvasID == uuid.Nil || current.InitialCanvasID != original.CanvasID {
			return application.ErrMainCanvasUnavailable
		}
		var document struct{ ID uuid.UUID }
		read = tx.Raw(`SELECT id FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, current.InitialCanvasID, current.ID).Scan(&document)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrMainCanvasUnavailable
		}
		result = application.ProjectCreationRecovery{State: "created", ProjectID: &current.ID, CanvasID: &current.InitialCanvasID, ProjectStatus: &current.Status}
		return nil
	})
	if err != nil {
		return application.ProjectCreationRecovery{}, fmt.Errorf("read creation recovery: %w", err)
	}
	return result, nil
}
