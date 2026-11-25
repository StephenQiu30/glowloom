package operation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProvidePromptCompiler preserves only the monolith's old quote port. Its exact
// session checks remain on the caller's transaction; independent Execution uses
// the authenticated Creation frozen-input protocol instead of this Go adapter.
func ProvidePromptCompiler(tx *gorm.DB) operation.QuotePromptCompiler {
	return legacyPromptCompiler{compiler: prompt.NewCompiler(ProvidePromptPreferenceStore(tx))}
}

type legacyPromptCompiler struct{ compiler *prompt.Compiler }

func (c legacyPromptCompiler) Prepare(ctx context.Context, actor identity.Principal, request prompt.TemplateRequest, runtime prompt.RuntimeContext) (prompt.Preparation, error) {
	out, err := c.compiler.Prepare(ctx, legacyPromptActor(actor), request, runtime)
	return out, legacyPromptError(err)
}

// ProvidePromptEditor retains the monolith's caller Tx while explicitly mapping
// its old account reference to Prompt's minimum actor at this assembly boundary.
func ProvidePromptEditor(tx *gorm.DB) operation.EditorSnapshots {
	return legacyPromptEditor{store: pgprompt.NewDraftStore(tx, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources)}
}

type legacyPromptEditor struct{ store *pgprompt.DraftStore }

func (e legacyPromptEditor) AuthorizeOwner(ctx context.Context, actor identity.Principal, project uuid.UUID, source promptdomain.InputSource) error {
	return legacyPromptError(e.store.AuthorizeOwner(ctx, legacyPromptActor(actor), project, source))
}
func (e legacyPromptEditor) ReadInput(ctx context.Context, actor identity.Principal, project uuid.UUID, source promptdomain.InputSource, confirm bool) (promptdomain.Draft, error) {
	out, err := e.store.ReadInput(ctx, legacyPromptActor(actor), project, source, confirm)
	return out, legacyPromptError(err)
}
func legacyPromptActor(actor identity.Principal) prompt.Actor {
	return prompt.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func legacyPromptError(err error) error {
	switch {
	case errors.Is(err, prompt.ErrSessionInvalid):
		return fmt.Errorf("%w: %w", identity.ErrSessionInvalid, err)
	case errors.Is(err, prompt.ErrForbidden):
		return fmt.Errorf("%w: %w", identity.ErrForbidden, err)
	default:
		return err
	}
}
