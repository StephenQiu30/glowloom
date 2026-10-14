package workspace_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type scopeFactsStore struct {
	facts application.ScopeFacts
	err   error
}

func (s scopeFactsStore) LoadScopeFacts(context.Context, uuid.UUID, uuid.UUID) (application.ScopeFacts, error) {
	return s.facts, s.err
}

func TestScopeAccessMatrix(t *testing.T) {
	actor, other, team, scope := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, tc := range []struct {
		name           string
		facts          application.ScopeFacts
		write, allowed bool
	}{
		{"own personal", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "personal", OwnerUserID: &actor}, ActorAvailable: true}, true, true},
		{"other personal", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "personal", OwnerUserID: &other}, ActorAvailable: true}, false, false},
		{"team member read", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "active", MemberStatus: "active", MemberRole: "member"}, false, true},
		{"team admin write", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "active", MemberStatus: "active", MemberRole: "team_admin"}, true, true},
		{"platform without membership", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "active"}, false, false},
		{"revoked member", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "active", MemberStatus: "removed", MemberRole: "team_admin"}, false, false},
		{"archived read", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "archived", MemberStatus: "active", MemberRole: "member"}, false, true},
		{"archived write", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "archived", MemberStatus: "active", MemberRole: "member"}, true, false},
		{"governance frozen write", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "active", GovernanceBlocked: true, MemberStatus: "active", MemberRole: "team_admin"}, true, false},
		{"suspended team", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "team", TeamID: &team}, ActorAvailable: true, TeamStatus: "suspended", MemberStatus: "active", MemberRole: "member"}, false, false},
		{"disabled account", application.ScopeFacts{Scope: domain.Scope{ID: scope, Kind: "personal", OwnerUserID: &actor}}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			access := application.NewScopeAccess(scopeFactsStore{facts: tc.facts})
			_, err := access.Resolve(t.Context(), actor, scope, tc.write)
			if (err == nil) != tc.allowed {
				t.Fatalf("access=%v, allowed=%v", err, tc.allowed)
			}
			if !tc.allowed && !errors.Is(err, application.ErrScopeNotFound) {
				t.Fatal("scope existence leaked")
			}
		})
	}
	failure := errors.New("authority database unavailable")
	if _, err := application.NewScopeAccess(scopeFactsStore{err: failure}).Resolve(t.Context(), actor, scope, false); !errors.Is(err, failure) {
		t.Fatal("authority error must fail closed, not fall back")
	}
}
