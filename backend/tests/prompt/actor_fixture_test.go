package prompt_test

import (
	"context"

	"github.com/gin-gonic/gin"

	promptpg "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

func testPromptActor(actor identity.Principal) promptapp.Actor {
	return promptapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func testLegacyPromptActor(actor promptapp.Actor) identity.Principal {
	return identity.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func promptHTTPActor(c *gin.Context) (promptapp.Actor, error) {
	return testPromptActor(identityhttp.Principal(c)), nil
}

type invalidPreferenceAuditStore struct{ *promptpg.Store }

func (s invalidPreferenceAuditStore) SaveCustomization(ctx context.Context, actor promptapp.Actor, input promptapp.SaveInput, desired domain.Customization, audit promptapp.PreferenceAudit) (domain.Customization, error) {
	// The real Outbox JSONB INSERT fails after the preference update in its real
	// PostgreSQL transaction. No fixture changes an existing database structure.
	audit.Payload = []byte("{")
	return s.Store.SaveCustomization(ctx, actor, input, desired, audit)
}
