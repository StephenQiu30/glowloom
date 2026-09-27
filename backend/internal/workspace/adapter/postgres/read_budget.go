package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// FindBudget requires a project ID and checks ownership through the project.
// The budget table has no cross-schema project foreign key.
func (s *Store) FindBudget(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (billingdomain.Budget, error) {
	if s == nil || s.db == nil {
		return billingdomain.Budget{}, ErrUnavailable
	}
	if projectID == uuid.Nil {
		return billingdomain.Budget{}, ErrBudgetNotFound
	}
	var budget billingdomain.Budget
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT b.id, b.project_id, b.limit_micros, b.reserved_micros,
			       b.settled_micros, b.is_overrun, b.revision
			FROM billing.budget AS b
			JOIN workspace.project AS p ON p.id = b.project_id
			WHERE p.id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT p.is_delete AND NOT b.is_delete
		`, projectID.String(), actor.OrgID.String()).Scan(&budget)
		if result.Error != nil {
			return fmt.Errorf("read project budget: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrBudgetNotFound
		}
		return nil
	})
	if err != nil {
		return billingdomain.Budget{}, fmt.Errorf("find project budget: %w", err)
	}
	return budget, nil
}
