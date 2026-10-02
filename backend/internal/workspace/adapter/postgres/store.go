// Package postgres persists organization scoped project records.
package postgres

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

var (
	// ErrProjectNotFound hides projects outside the current organization.
	ErrProjectNotFound = application.ErrProjectNotFound
	// ErrStylePresetNotFound hides unavailable presets outside the current scope.
	ErrStylePresetNotFound = application.ErrStylePresetNotFound
	// ErrStylePresetMismatch means a visible preset does not match the project style.
	ErrStylePresetMismatch = application.ErrStylePresetMismatch
	// ErrBudgetNotFound hides budgets outside the current project scope.
	ErrBudgetNotFound = errors.New("budget not found")
	// ErrUnavailable means the store was constructed without a database handle.
	ErrUnavailable = errors.New("workspace store unavailable")
)

// Store keeps project persistence behind an explicitly injected database handle.
type Store struct {
	db    *gorm.DB
	work  ProjectWorkFactory
	cover ProjectCoverFactory
}

// NewStore creates a project store using the caller's database handle.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// requireCurrentActor locks the live account and organization for a transaction.
// An authenticated principal is not sufficient if either has since been disabled.
func requireCurrentActor(tx *gorm.DB, actor identityapp.Principal) error {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return identityapp.ErrForbidden
	}
	var present int
	result := tx.Raw(`
		SELECT 1
		FROM identity."user" AS u
		JOIN workspace.organization AS o ON o.id = u.org_id
		WHERE u.id = ?::uuid AND u.org_id = ?::uuid
		  AND u.role IN ('admin', 'producer') AND u.role = ?
		  AND u.status = 'active' AND NOT u.is_delete AND NOT u.must_change_password
		  AND o.status = 'active' AND NOT o.is_delete
		FOR SHARE OF u, o
	`, actor.ID.String(), actor.OrgID.String(), string(actor.Role)).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check current project actor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	return nil
}
