// Package postgres reads project scoped budget and ledger records.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrNotFound hides records outside the current project or organization.
	ErrNotFound = errors.New("billing record not found")
	// ErrUnavailable means the store has no database handle.
	ErrUnavailable = errors.New("billing store unavailable")
)

// Store keeps the database handle injected by the application composition root.
type Store struct{ db *gorm.DB }

// NewStore injects the caller's database handle.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// FindBudget checks the current account and project owner in the database.
func (s *Store) FindBudget(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (domain.Budget, error) {
	if s == nil || s.db == nil {
		return domain.Budget{}, ErrUnavailable
	}
	if projectID == uuid.Nil {
		return domain.Budget{}, ErrNotFound
	}
	var budget domain.Budget
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT b.id, b.project_id, b.limit_micros, b.reserved_micros,
			       b.settled_micros, b.is_overrun, b.revision
			FROM billing.budget AS b
			JOIN workspace.project AS p ON p.id = b.project_id
			WHERE b.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT b.is_delete AND NOT p.is_delete
		`, projectID.String(), actor.OrgID.String()).Scan(&budget)
		if result.Error != nil {
			return fmt.Errorf("read budget: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return budget.Validate()
	})
	if err != nil {
		return domain.Budget{}, fmt.Errorf("find budget: %w", err)
	}
	return budget, nil
}

// FindLedgerEntry always scopes the ID by both project and current organization.
func (s *Store) FindLedgerEntry(ctx context.Context, actor identityapp.Principal, projectID, entryID uuid.UUID) (domain.LedgerEntry, error) {
	if s == nil || s.db == nil {
		return domain.LedgerEntry{}, ErrUnavailable
	}
	if projectID == uuid.Nil || entryID == uuid.Nil {
		return domain.LedgerEntry{}, ErrNotFound
	}
	var entry domain.LedgerEntry
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT l.id, l.project_id, l.entry_type, l.amount_micros,
			       l.operation_id, l.episode_id, l.shot_id, l.model_key, l.region,
			       l.orig_currency, l.orig_amount_micros, l.fx_rate::text AS fx_rate,
			       l.note, l.create_time, l.create_by
			FROM billing.ledger_entry AS l
			JOIN workspace.project AS p ON p.id = l.project_id
			WHERE l.id = ?::uuid AND l.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT l.is_delete AND NOT p.is_delete
		`, entryID.String(), projectID.String(), actor.OrgID.String()).Scan(&entry)
		if result.Error != nil {
			return fmt.Errorf("read ledger entry: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("find ledger entry: %w", err)
	}
	return entry, nil
}

func requireCurrentActor(tx *gorm.DB, actor identityapp.Principal) error {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return identityapp.ErrForbidden
	}
	var present int
	result := tx.Raw(`
		SELECT 1
		FROM identity."user" AS u
		JOIN workspace.organization AS o ON o.id = u.org_id
		WHERE u.id = ?::uuid AND u.org_id = ?::uuid
		  AND u.role IN ('admin', 'producer')
		  AND u.status = 'active' AND NOT u.is_delete AND NOT u.must_change_password
		  AND o.status = 'active' AND NOT o.is_delete
		FOR SHARE OF u, o
	`, actor.ID.String(), actor.OrgID.String()).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check current billing actor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	return nil
}
