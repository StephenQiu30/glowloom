package operation_test

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	pgprompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

func operationPromptActor(actor identity.Principal) prompt.Actor {
	return prompt.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func operationPromptIdentity(actor prompt.Actor) identity.Principal {
	return identity.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func operationPromptHTTPActor(c *gin.Context) (prompt.Actor, error) {
	return operationPromptActor(identityhttp.Principal(c)), nil
}

// These retained monolith integration tests keep the old Operation port while
// the actual Prompt store and its authorization receive only Prompt's own actor.
type operationEditorPromptStore struct{ *pgprompt.DraftStore }

func (s *operationEditorPromptStore) SaveDraft(ctx context.Context, actor identity.Principal, input prompt.DraftWrite) (prompt.DraftSnapshot, error) {
	return s.DraftStore.SaveDraft(ctx, operationPromptActor(actor), input)
}
func (s *operationEditorPromptStore) ReadInput(ctx context.Context, actor identity.Principal, project uuid.UUID, source domain.InputSource, confirm bool) (domain.Draft, error) {
	return s.DraftStore.ReadInput(ctx, operationPromptActor(actor), project, source, confirm)
}
func (s *operationEditorPromptStore) AuthorizeOwner(ctx context.Context, actor identity.Principal, project uuid.UUID, source domain.InputSource) error {
	return s.DraftStore.AuthorizeOwner(ctx, operationPromptActor(actor), project, source)
}
