package postgres

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ProjectAccessFactory keeps account, scope and project locks in the canvas transaction.
type ProjectAccessFactory func(*gorm.DB) application.ProjectAccess

type projectAuthority struct{ project ProjectAccessFactory }

func (s projectAuthority) presentation(tx *gorm.DB, actor identity.Principal, id uuid.UUID) (application.ProjectPresentation, error) {
	if tx == nil || s.project == nil {
		return application.ProjectPresentation{}, application.ErrNotFound
	}
	access := s.project(tx)
	if access == nil {
		return application.ProjectPresentation{}, application.ErrNotFound
	}
	return access.ReadProjectPresentation(tx.Statement.Context, actor, id)
}

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

// readAuthority takes the strongest required lock first, then positively checks
// read access when a project is archived or the current scope is read-only.
func (s projectAuthority) readAuthority(tx *gorm.DB, actor identity.Principal, id uuid.UUID) (bool, error) {
	err := s.authorize(tx, actor, id, true)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, identity.ErrForbidden) && !errors.Is(err, mediaapp.ErrNotFound) && !errors.Is(err, mediadomain.ErrMediaStateConflict) {
		return false, err
	}
	return false, s.authorize(tx, actor, id, false)
}
