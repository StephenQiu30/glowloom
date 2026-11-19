package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// CallModel resolves an existing logical request before looking at a mutable alias.
// Admit still verifies the entire submitted input and current caller authority.
func (s *Store) CallModel(ctx context.Context, scope, run uuid.UUID, key, origin, name, protocol string) (domain.Model, error) {
	var model domain.Model
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		var row struct{ Contract []byte }
		q := tx.Raw("SELECT contract FROM ai.request WHERE scope_id=? AND actor_id=? AND run_id IS NOT DISTINCT FROM ? AND idempotency_key=? AND origin=?", scope, s.actor.ID, nullable(run), key, origin).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if json.Unmarshal(row.Contract, &model) != nil || model.Name != name || model.Protocol != protocol {
				return domain.ErrConflict
			}
			return nil
		}
		var err error
		model, err = s.Model(ctx, name, protocol)
		return err
	})
	return model, err
}

// RevokeGrant stops new SDK admission without changing already acquired evidence.
func (s *Store) RevokeGrant(ctx context.Context, scope, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		r := tx.Exec("UPDATE ai.integration_grant SET revoked=true WHERE id=? AND scope_id=? AND actor->>'ID'=?", id, scope, s.actor.ID.String())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrForbidden
		}
		return nil
	})
}

// GrantModels reads only the immutable models explicitly permitted by this grant.
func (s *Store) GrantModels(ctx context.Context, scope uuid.UUID) ([]domain.Model, error) {
	result := []domain.Model{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		var rows []struct{ Document []byte }
		if err := tx.Raw(`SELECT m.document FROM ai.model m JOIN ai.integration_grant g ON g.model_ids @> jsonb_build_array(m.id::text) WHERE g.id=? AND g.scope_id=? AND NOT m.draft AND NOT g.revoked AND g.expires_at>clock_timestamp() ORDER BY m.name,m.id`, s.grant, scope).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var m domain.Model
			if json.Unmarshal(row.Document, &m) != nil {
				return domain.ErrInvalid
			}
			result = append(result, m)
		}
		return nil
	})
	return result, err
}
