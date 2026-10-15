package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// PersonalScopeID reads a mapping, not authority. Callers must subsequently Resolve
// this exact scope in the same transaction before using any personal resource.
func (s *ScopeStore) PersonalScopeID(ctx context.Context, actor uuid.UUID) (uuid.UUID, error) {
	if s == nil || s.db == nil || s.db.Statement == nil || s.actors == nil {
		return uuid.Nil, application.ErrScopeNotFound
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return uuid.Nil, application.ErrScopeNotFound
	}
	if err := s.actors(s.db).CheckActive(ctx, actor); err != nil {
		return uuid.Nil, err
	}
	var row struct{ ID uuid.UUID }
	r := s.db.WithContext(ctx).Raw(`SELECT id FROM workspace.creation_scope WHERE kind='personal' AND owner_user_id=? FOR SHARE`, actor).Scan(&row)
	if r.Error != nil {
		return uuid.Nil, r.Error
	}
	if r.RowsAffected != 1 {
		return uuid.Nil, application.ErrScopeNotFound
	}
	return row.ID, nil
}
