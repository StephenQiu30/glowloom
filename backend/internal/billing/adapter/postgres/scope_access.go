package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectAccessFactory retains current authority locks in the billing transaction.
type ProjectAccessFactory func(*gorm.DB) application.ProjectAccess

// NewScopedStore injects project access, including copying-target access for the coordinator.
func NewScopedStore(db *gorm.DB, access ProjectAccessFactory) *Store {
	return &Store{db: db, access: access}
}

func (s *Store) requireProject(tx *gorm.DB, actor identity.Principal, project uuid.UUID, write bool) error {
	if s.access == nil || s.access(tx) == nil {
		return identity.ErrForbidden
	}
	return s.access(tx).AuthorizeBilling(tx.Statement.Context, actor, project, write)
}
