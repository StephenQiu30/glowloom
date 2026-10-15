package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// RequestScopeFactory binds request checks to the consumer transaction.
type RequestScopeFactory func(*gorm.DB) application.RequestScopeResolver

// NewScopedStore requires current session and scope checks for project requests.
func NewScopedStore(db *gorm.DB, scopes RequestScopeFactory, work ProjectWorkFactory, cover ProjectCoverFactory) *Store {
	return &Store{db: db, scopes: scopes, work: work, cover: cover}
}

func (s *Store) requireScope(ctx context.Context, tx *gorm.DB, actor identity.Principal, write bool) error {
	if s.scopes == nil {
		return application.ErrProjectDependencyUnavailable
	}
	authority := s.scopes(tx)
	if authority == nil {
		return application.ErrProjectDependencyUnavailable
	}
	scope, err := authority.Resolve(ctx, actor, actor.ScopeID, write)
	if err != nil {
		return err
	}
	if scope.ID != actor.ScopeID || scope.PhysicalScopeID != actor.OrgID {
		return application.ErrScopeNotFound
	}
	return nil
}

// authorizeProject deliberately includes deleted and archived objects here: each
// lifecycle command separately decides whether that state allows its transition.
func (s *Store) authorizeProject(ctx context.Context, tx *gorm.DB, actor identity.Principal, project uuid.UUID, write bool) error {
	if err := s.requireScope(ctx, tx, actor, write); err != nil {
		return err
	}
	var present int
	query := `SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=?`
	// Take the strongest required lock on the first project read: acquiring a
	// shared lock here then upgrading in the command allows concurrent writers
	// to deadlock while each retains the other's shared lock.
	if write {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	read := tx.Raw(query, project, actor.OrgID, actor.ScopeID).Scan(&present)
	if read.Error != nil {
		return fmt.Errorf("authorize project scope: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.ErrProjectNotFound
	}
	return nil
}
