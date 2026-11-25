package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// ErrRouteVerifying indicates that a requested channel is being verified.
var ErrRouteVerifying = errors.New("route verification in progress")

// ErrRouteNotFound indicates that a requested channel does not exist.
var ErrRouteNotFound = errors.New("route not found")

// DeleteRoutes retires channels and their onboarding entries without rewriting historical contracts.
func (s *Store) DeleteRoutes(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 || len(ids) > 1100 {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		found := make(map[uuid.UUID]bool, len(ids))
		// ConfirmOnboarding holds the candidate before creating its route. Use the same order.
		var candidates []struct {
			ID    uuid.UUID
			State string
		}
		if err := tx.Raw("SELECT id,state FROM ai.onboarding WHERE id IN ? ORDER BY id FOR UPDATE", ids).Scan(&candidates).Error; err != nil {
			return err
		}
		for _, row := range candidates {
			if row.State == "verifying" {
				return ErrRouteVerifying
			}
			found[row.ID] = true
		}
		var routes []uuid.UUID
		if err := tx.Raw("SELECT id FROM ai.route WHERE id IN ? ORDER BY id FOR UPDATE", ids).Scan(&routes).Error; err != nil {
			return err
		}
		for _, id := range routes {
			found[id] = true
		}
		for _, id := range ids {
			if id == uuid.Nil || !found[id] {
				return ErrRouteNotFound
			}
		}
		if err := tx.Exec("UPDATE ai.onboarding SET deleted_at=transaction_timestamp(),deleted_by=? WHERE id IN ? AND deleted_at IS NULL", s.actor.ID, ids).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE ai.route SET enabled=false,deleted_at=transaction_timestamp(),deleted_by=? WHERE id IN ? AND deleted_at IS NULL", s.actor.ID, ids).Error
	})
}
