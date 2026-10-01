package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// CreateCopyTargetBudget creates an independent zero budget for one unpublished target.
// The coordinator injects its transaction; source financial facts are never queried.
func (s *Store) CreateCopyTargetBudget(ctx context.Context, actor identityapp.Principal, target uuid.UUID) error {
	if s == nil || s.db == nil || target == uuid.Nil {
		return ErrUnavailable
	}
	tx := s.db.WithContext(ctx)
	if err := requireCurrentActor(tx, actor); err != nil {
		return err
	}
	var present int
	read := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete FOR SHARE`, target, actor.OrgID).Scan(&present)
	if read.Error != nil {
		return fmt.Errorf("authorize copied project budget: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return ErrNotFound
	}
	if err := tx.Exec(`INSERT INTO billing.budget(id,project_id,limit_micros,reserved_micros,settled_micros,is_overrun,revision) VALUES(?,?,0,0,0,false,1)`, uuid.New(), target).Error; err != nil {
		return fmt.Errorf("create independent copy budget: %w", err)
	}
	return nil
}

// VerifyCopyTargetBudget proves the unpublished copy still has its independent zero budget.
func (s *Store) VerifyCopyTargetBudget(ctx context.Context, actor identityapp.Principal, target uuid.UUID) error {
	if s == nil || s.db == nil || target == uuid.Nil {
		return ErrUnavailable
	}
	if err := requireCurrentActor(s.db.WithContext(ctx), actor); err != nil {
		return err
	}
	var valid bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM billing.budget b JOIN workspace.project p ON p.id=b.project_id WHERE p.id=? AND p.org_id=? AND p.status='copying' AND NOT p.is_delete AND NOT b.is_delete AND b.limit_micros=0 AND b.reserved_micros=0 AND b.settled_micros=0 AND NOT b.is_overrun)`, target, actor.OrgID).Scan(&valid).Error
	if err != nil {
		return err
	}
	if !valid {
		return ErrNotFound
	}
	return nil
}
