package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ReadSettlementFacts reads only financial metadata and lifecycle status for the internal finalizer.
// It grants no content access and allows existing liabilities to close after revocation.
func (s *Store) ReadSettlementFacts(ctx context.Context, id uuid.UUID) (application.SettlementFacts, error) {
	if s == nil {
		return application.SettlementFacts{}, ErrUnavailable
	}
	return s.ReadSettlementFactsInTransaction(ctx, s.db, id)
}

// ReadSettlementFactsInTransaction keeps financial metadata reads on the
// caller's connection, including when the Store was built from the root pool.
func (s *Store) ReadSettlementFactsInTransaction(ctx context.Context, tx *gorm.DB, id uuid.UUID) (application.SettlementFacts, error) {
	var result application.SettlementFacts
	if s == nil || s.db == nil || tx == nil {
		return result, ErrUnavailable
	}
	r := tx.WithContext(ctx).Raw(`SELECT o.status,o.project_id,o.capability,o.region,m.model_key,pr.unit AS price_unit FROM operation.operation o LEFT JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id LEFT JOIN catalog.model_profile m ON m.id=v.model_profile_id LEFT JOIN catalog.price_rule_version pr ON pr.id=o.price_rule_version_id WHERE o.id=? AND NOT o.is_delete`, id).Scan(&result)
	if r.Error != nil {
		return result, fmt.Errorf("read settlement facts: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return result, ErrNotFound
	}
	return result, nil
}
