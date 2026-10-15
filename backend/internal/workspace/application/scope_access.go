package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// ErrScopeNotFound deliberately hides private scope existence.
var ErrScopeNotFound = errors.New("creation scope not found")

// ActorAccess checks live account eligibility without granting a content role.
type ActorAccess interface {
	CheckActive(context.Context, uuid.UUID) error
}

// ScopeFacts combines current ownership, team and membership facts.
type ScopeFacts struct {
	Scope                                domain.Scope
	ActorAvailable                       bool
	TeamStatus, MemberStatus, MemberRole string
	GovernanceBlocked                    bool
}

// ScopeFactsStore loads scope authorization facts without trusting request roles.
type ScopeFactsStore interface {
	LoadScopeFacts(context.Context, uuid.UUID, uuid.UUID) (ScopeFacts, error)
}

// ScopeAccess authorizes ownership or active membership independently of platform roles.
type ScopeAccess struct{ store ScopeFactsStore }

// NewScopeAccess constructs ScopeAccess with explicit caller-supplied dependencies.
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
	return resolveScopeFacts(facts, actor, scope, write)
}

func resolveScopeFacts(facts ScopeFacts, actor, scope uuid.UUID, write bool) (domain.Scope, error) {
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

// SessionActorAccess is distinct from worker availability checks: requests must
// retain their exact session and credential epoch until the content commit.
type SessionActorAccess interface {
	ActorAccess
	CheckSession(context.Context, identity.Principal, func() time.Time) error
}

// RequestScopeFactsStore rechecks the exact session before loading scope facts.
type RequestScopeFactsStore interface {
	LoadRequestScopeFacts(context.Context, identity.Principal, uuid.UUID, func() time.Time) (ScopeFacts, error)
}

// RequestScopeAccess binds content scope authorization to a live request session.
type RequestScopeAccess struct {
	store RequestScopeFactsStore
	now   func() time.Time
}

// NewRequestScopeAccess constructs RequestScopeAccess with explicit caller-supplied dependencies.
func NewRequestScopeAccess(store RequestScopeFactsStore, now func() time.Time) *RequestScopeAccess {
	return &RequestScopeAccess{store: store, now: now}
}

// Resolve requires the exact live session before evaluating scope membership.
func (s *RequestScopeAccess) Resolve(ctx context.Context, actor identity.Principal, scope uuid.UUID, write bool) (domain.Scope, error) {
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 || scope == uuid.Nil {
		return domain.Scope{}, ErrScopeNotFound
	}
	facts, err := s.store.LoadRequestScopeFacts(ctx, actor, scope, s.now)
	if err != nil {
		return domain.Scope{}, err
	}
	return resolveScopeFacts(facts, actor.ID, scope, write)
}

// RequestScopeResolver retains exact-session and membership locks in the caller transaction.
type RequestScopeResolver interface {
	Resolve(context.Context, identity.Principal, uuid.UUID, bool) (domain.Scope, error)
}

// WorkerScopeResolver checks live account and membership without inventing a request session.
type WorkerScopeResolver interface {
	Resolve(context.Context, uuid.UUID, uuid.UUID, bool) (domain.Scope, error)
}
