package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// RunLimit reads a live caller-owned authorization, after account locking and before new funds admission.
type RunLimit func(context.Context, *gorm.DB, uuid.UUID) (int64, error)

// WithRunLimit binds a worker's current approved limit. It grants no scope or send authority.
func (s *Store) WithRunLimit(limit RunLimit) *Store {
	bound := *s
	bound.runLimit = limit
	return &bound
}
func (s *Store) syncRunLimit(ctx context.Context, tx *gorm.DB, call domain.Call) error {
	if s.runLimit == nil || call.Run == uuid.Nil {
		return nil
	}
	limit, err := s.runLimit(ctx, tx, call.Run)
	if err != nil {
		return err
	}
	if limit < 0 {
		return domain.ErrBudget
	}
	if err := tx.Exec("INSERT INTO ai.budget(scope_id,id,limit_credits) VALUES(?,?,?) ON CONFLICT DO NOTHING", call.Scope, call.Run, limit).Error; err != nil {
		return err
	}
	result := tx.Exec("UPDATE ai.budget SET limit_credits=? WHERE scope_id=? AND id=? AND held+spent<=?", limit, call.Scope, call.Run, limit)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrBudget
	}
	return nil
}
