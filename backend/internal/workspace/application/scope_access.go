package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// ErrScopeNotFound deliberately hides private scope existence.
var ErrScopeNotFound = errors.New("creation scope not found")

type ActorAccess interface {
	CheckActive(context.Context, uuid.UUID) error
}
type ScopeFacts struct {
	Scope                                domain.Scope
	ActorAvailable                       bool
	TeamStatus, MemberStatus, MemberRole string
	GovernanceBlocked                    bool
}
type ScopeFactsStore interface {
	LoadScopeFacts(context.Context, uuid.UUID, uuid.UUID) (ScopeFacts, error)
}
type ScopeAccess struct{ store ScopeFactsStore }

func NewScopeAccess(store ScopeFactsStore) *ScopeAccess { return &ScopeAccess{store: store} }

// Resolve grants content access solely from ownership or active membership.
// Platform governance authority is intentionally absent from this interface.
func (s *ScopeAccess) Resolve(ctx context.Context, actor, scope uuid.UUID, write bool) (domain.Scope, error) {
	if actor == uuid.Nil || scope == uuid.Nil {
		return domain.Scope{}, ErrScopeNotFound
	}
	facts, err := s.store.LoadScopeFacts(ctx, actor, scope)
	if err != nil {
		return domain.Scope{}, err
	}
	if !facts.ActorAvailable || facts.Scope.ID != scope {
		return domain.Scope{}, ErrScopeNotFound
	}
	switch facts.Scope.Kind {
	case "personal":
		if facts.Scope.OwnerUserID == nil || *facts.Scope.OwnerUserID != actor || facts.Scope.TeamID != nil {
			return domain.Scope{}, ErrScopeNotFound
		}
	case "team":
		if facts.Scope.TeamID == nil || *facts.Scope.TeamID == uuid.Nil || facts.Scope.OwnerUserID != nil || facts.MemberStatus != "active" || (facts.MemberRole != "member" && facts.MemberRole != "team_admin") || (facts.TeamStatus != "active" && facts.TeamStatus != "archived") || (write && (facts.TeamStatus != "active" || facts.GovernanceBlocked)) {
			return domain.Scope{}, ErrScopeNotFound
		}
	default:
		return domain.Scope{}, ErrScopeNotFound
	}
	return facts.Scope, nil
}
