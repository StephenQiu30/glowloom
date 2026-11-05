package agent

import (
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
)

// ConversationInputs wires the formal draft, catalog and quote owners for Agent input admission.
func (b *Boundary) ConversationInputs(quotes app.QuoteReader) *app.ConversationInputService {
	drafts := pgprompt.NewDraftStore(b.db, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources)
	return app.NewConversationInputService(drafts, catalogStore(b.db), quotes)
}

// ConversationHistory wires current draft source authority for historical conversation reads.
func (b *Boundary) ConversationHistory() *app.ConversationHistory {
	drafts := pgprompt.NewDraftStore(b.db, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources)
	return app.NewConversationHistory(drafts)
}

// ConversationSources supplies read authority without replaying an accepted input.
func (b *Boundary) ConversationSources() app.ConversationSourceReader {
	return pgprompt.NewDraftStore(b.db, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources)
}
