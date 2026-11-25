package agentbusiness_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestScopePrincipalPreservesOnlyEstablishedIdentity(t *testing.T) {
	for _, scope := range []domain.Scope{
		{},
		{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New(), CanvasID: new(uuid.New())},
	} {
		got := application.Principal(scope)
		want := identity.Principal{ID: scope.ActorID, OrgID: scope.OrgID, ScopeID: scope.ScopeID}
		if got != want {
			t.Fatalf("scope conversion changed identity or added authority: got %+v, want %+v", got, want)
		}
	}
}
