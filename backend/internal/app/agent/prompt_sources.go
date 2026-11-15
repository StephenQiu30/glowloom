package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	pgprompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

type conversationPromptSource struct{ store *pgprompt.DraftStore }

func (s conversationPromptSource) FreezeConversationInput(ctx context.Context, actor identity.Principal, project uuid.UUID, draft domain.Draft) (domain.ConversationInput, error) {
	out, err := s.store.FreezeConversationInput(ctx, conversationPromptActor(actor), project, draft)
	return out, conversationPromptError(err)
}
func (s conversationPromptSource) CheckConversationSources(ctx context.Context, actor identity.Principal, project uuid.UUID, draft domain.Draft) error {
	return conversationPromptError(s.store.CheckConversationSources(ctx, conversationPromptActor(actor), project, draft))
}
func conversationPromptActor(actor identity.Principal) prompt.Actor {
	return prompt.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func conversationPromptError(err error) error {
	switch {
	case errors.Is(err, prompt.ErrSessionInvalid):
		return fmt.Errorf("%w: %w", identity.ErrSessionInvalid, err)
	case errors.Is(err, prompt.ErrForbidden):
		return fmt.Errorf("%w: %w", identity.ErrForbidden, err)
	default:
		return err
	}
}
