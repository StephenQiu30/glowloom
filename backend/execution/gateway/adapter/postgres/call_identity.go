package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

func verifyCallIdentity(tx *gorm.DB, call domain.Call) error {
	var row struct {
		ScopeID, ActorID uuid.UUID
		ProjectID, RunID *uuid.UUID
		InputHash        string
	}
	q := tx.Raw("SELECT scope_id,actor_id,project_id,run_id,input_hash FROM ai.request WHERE id=?", call.ID).Scan(&row)
	if q.Error != nil {
		return q.Error
	}
	project, run := uuid.Nil, uuid.Nil
	if row.ProjectID != nil {
		project = *row.ProjectID
	}
	if row.RunID != nil {
		run = *row.RunID
	}
	if q.RowsAffected != 1 || row.ScopeID != call.Scope || row.ActorID != call.Actor || project != call.Project || run != call.Run || row.InputHash != call.Hash {
		return domain.ErrConflict
	}
	return nil
}
