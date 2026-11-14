package application

import (
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// Principal translates a server-established scope for an Identity consumer.
// It does not authenticate the caller or add session or platform authority.
func Principal(scope domain.Scope) identity.Principal {
	return identity.Principal{ID: scope.ActorID, OrgID: scope.OrgID, ScopeID: scope.ScopeID}
}
