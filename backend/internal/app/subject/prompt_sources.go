package subject

import (
	"context"

	"github.com/google/uuid"

	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// The retained monolith maps Prompt's minimum reference here while preserving
// the original caller transaction and its current session/scope policy locks.
func subjectPromptIdentity(actor prompt.Actor) identity.Principal {
	return identity.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}
func (g generationAccess) ResolveSubject(ctx context.Context, actor prompt.Actor, project uuid.UUID, id, version string) (domain.ReferenceSource, bool, error) {
	out, known, err := g.resolvePromptSubject(ctx, subjectPromptIdentity(actor), project, id, version)
	return out, known, promptwire.AccessError(err)
}
func (g generationAccess) ValidateBinding(ctx context.Context, actor prompt.Actor, project uuid.UUID, media domain.SourceMedia, ready bool) error {
	return promptwire.AccessError(g.validatePromptBinding(ctx, subjectPromptIdentity(actor), project, media, ready))
}
func (g generationAccess) ResolveCanvasSubject(ctx context.Context, actor prompt.Actor, project uuid.UUID, id, version string) (domain.ReferenceSource, error) {
	out, err := g.resolvePromptCanvasSubject(ctx, subjectPromptIdentity(actor), project, id, version)
	return out, promptwire.AccessError(err)
}
func (s canvasCopySources) ResolveSubject(ctx context.Context, actor prompt.Actor, project uuid.UUID, id, version string) (domain.ReferenceSource, bool, error) {
	out, known, err := s.resolvePromptSubject(ctx, subjectPromptIdentity(actor), project, id, version)
	return out, known, promptwire.AccessError(err)
}
func (s canvasCopySources) ValidateBinding(ctx context.Context, actor prompt.Actor, project uuid.UUID, media domain.SourceMedia, ready bool) error {
	return promptwire.AccessError(s.validatePromptBinding(ctx, subjectPromptIdentity(actor), project, media, ready))
}
