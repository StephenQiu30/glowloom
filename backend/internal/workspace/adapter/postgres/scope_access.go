package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// ActorAccessFactory binds current-account checks to the content transaction.
type ActorAccessFactory func(*gorm.DB) application.ActorAccess

// ScopeStore loads durable ownership and membership facts under row locks.
type ScopeStore struct {
	db       *gorm.DB
	actors   ActorAccessFactory
	sessions SessionActorAccessFactory
}

// SessionActorAccessFactory binds request-session checks to the content transaction.
type SessionActorAccessFactory func(*gorm.DB) application.SessionActorAccess

// NewRequestScopeStore constructs RequestScopeStore with explicit caller-supplied dependencies.
func NewRequestScopeStore(db *gorm.DB, sessions SessionActorAccessFactory) *ScopeStore {
	return &ScopeStore{db: db, sessions: sessions, actors: func(tx *gorm.DB) application.ActorAccess { return sessions(tx) }}
}

// LoadRequestScopeFacts retains session and scope locks until the outer transaction ends.
func (s *ScopeStore) LoadRequestScopeFacts(ctx context.Context, actor identity.Principal, scope uuid.UUID, now time.Time) (application.ScopeFacts, error) {
	var facts application.ScopeFacts
	if s.sessions == nil {
		return facts, application.ErrScopeNotFound
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.sessions(tx).CheckSession(ctx, actor, now); err != nil {
			return err
		}
		var err error
		facts, err = NewScopeStore(tx, s.actors).LoadScopeFacts(ctx, actor.ID, scope)
		return err
	})
	return facts, err
}

// NewScopeStore constructs ScopeStore with explicit caller-supplied dependencies.
func NewScopeStore(db *gorm.DB, actors ActorAccessFactory) *ScopeStore {
	return &ScopeStore{db: db, actors: actors}
}

// LoadScopeFacts retains share locks until the caller's outer content transaction
// completes when constructed with its transaction handle.
func (s *ScopeStore) LoadScopeFacts(ctx context.Context, actor, scope uuid.UUID) (application.ScopeFacts, error) {
	var facts application.ScopeFacts
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.actors(tx).CheckActive(ctx, actor); err != nil {
			return err
		}
		facts.ActorAvailable = true
		r := tx.Raw(`SELECT id,kind,owner_user_id,team_id,physical_scope_id FROM workspace.creation_scope WHERE id=? FOR SHARE`, scope).Scan(&facts.Scope)
		if r.Error != nil {
			return fmt.Errorf("read creation scope: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return application.ErrScopeNotFound
		}
		if facts.Scope.Kind == "personal" {
			return nil
		}
		if facts.Scope.TeamID == nil {
			return application.ErrScopeNotFound
		}
		var team struct {
			Status            string
			GovernanceBlocked bool
		}
		r = tx.Raw(`SELECT status,governance_blocked FROM workspace.team WHERE id=? FOR SHARE`, *facts.Scope.TeamID).Scan(&team)
		if r.Error != nil {
			return fmt.Errorf("read scope team: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return application.ErrScopeNotFound
		}
		facts.TeamStatus, facts.GovernanceBlocked = team.Status, team.GovernanceBlocked
		var member struct{ Status, Role string }
		r = tx.Raw(`SELECT status,role FROM workspace.team_member WHERE team_id=? AND user_id=? FOR SHARE`, *facts.Scope.TeamID, actor).Scan(&member)
		if r.Error != nil {
			return fmt.Errorf("read scope membership: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return application.ErrScopeNotFound
		}
		facts.MemberStatus, facts.MemberRole = member.Status, member.Role
		return nil
	})
	return facts, err
}
