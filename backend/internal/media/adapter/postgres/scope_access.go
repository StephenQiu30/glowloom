package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// ScopeAccessFactory binds the explicitly selected request or worker authority to a transaction.
type ScopeAccessFactory func(*gorm.DB) application.ScopeAccess

type scopeAuthority struct{ scopes ScopeAccessFactory }

func (s scopeAuthority) accessScope(tx *gorm.DB) (application.ScopeAccess, error) {
	if tx == nil || tx.Statement == nil || s.scopes == nil {
		return nil, application.ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, application.ErrUnavailable
	}
	a := s.scopes(tx)
	if a == nil {
		return nil, application.ErrUnavailable
	}
	return a, nil
}
func (s scopeAuthority) requireCurrentActor(tx *gorm.DB, actor identity.Principal) error {
	a, err := s.accessScope(tx)
	if err != nil {
		return err
	}
	return a.CheckActor(tx.Statement.Context, actor)
}
func (s scopeAuthority) requireProject(tx *gorm.DB, actor identity.Principal, id uuid.UUID, write bool) error {
	a, err := s.accessScope(tx)
	if err != nil {
		return err
	}
	return a.AuthorizeProject(tx.Statement.Context, actor, id, write)
}
func (s scopeAuthority) requirePersonal(tx *gorm.DB, actor identity.Principal, write bool) error {
	a, err := s.accessScope(tx)
	if err != nil {
		return err
	}
	return a.AuthorizePersonal(tx.Statement.Context, actor, write)
}
func (s scopeAuthority) requireCopyTarget(tx *gorm.DB, actor identity.Principal, id uuid.UUID) error {
	a, err := s.accessScope(tx)
	if err != nil {
		return err
	}
	return a.AuthorizeCopyTarget(tx.Statement.Context, actor, id)
}
