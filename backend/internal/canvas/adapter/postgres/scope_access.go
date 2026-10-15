package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectAccessFactory keeps account, scope and project locks in the canvas transaction.
type ProjectAccessFactory func(*gorm.DB) application.ProjectAccess

type projectAuthority struct{ project ProjectAccessFactory }

func (s projectAuthority) authorize(tx *gorm.DB, actor identity.Principal, id uuid.UUID, write bool) error {
	if tx == nil || s.project == nil {
		return application.ErrNotFound
	}
	a := s.project(tx)
	if a == nil {
		return application.ErrNotFound
	}
	return a.AuthorizeProject(tx.Statement.Context, actor, id, write)
}
func (s projectAuthority) authorizeCopy(tx *gorm.DB, actor identity.Principal, id uuid.UUID) error {
	if tx == nil || s.project == nil {
		return application.ErrNotFound
	}
	a := s.project(tx)
	if a == nil {
		return application.ErrNotFound
	}
	return a.AuthorizeCopyTarget(tx.Statement.Context, actor, id)
}
