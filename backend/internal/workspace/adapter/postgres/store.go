// Package postgres persists organization scoped project records.
package postgres

import (
	"errors"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
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
	db     *gorm.DB
	scopes RequestScopeFactory
	work   ProjectWorkFactory
	cover  ProjectCoverFactory
}

// NewStore has no request authority and fails closed for project requests.
// NewScopedStore must be used by content consumers.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }
