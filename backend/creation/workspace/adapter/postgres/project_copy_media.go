package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func freezeProjectCopyMedia(ctx context.Context, actor identityapp.Principal, owner application.ProjectCopyMediaOwner, binding mediaapp.ProjectCopyBinding, at time.Time, references []uuid.UUID) (mediaapp.ProjectCopySnapshot, error) {
	if len(references) == 0 {
		return owner.Freeze(ctx, projectCopyMediaActor(actor), binding, at)
	}
	if len(references) > 4096 {
		return mediaapp.ProjectCopySnapshot{}, domain.ErrInvalidProjectCopy
	}
	seen := make(map[uuid.UUID]bool, len(references))
	for _, id := range references {
		if id == uuid.Nil || seen[id] {
			return mediaapp.ProjectCopySnapshot{}, domain.ErrInvalidProjectCopy
		}
		seen[id] = true
	}
	historical, ok := owner.(application.ProjectCopyHistoricalMediaOwner)
	if !ok {
		return mediaapp.ProjectCopySnapshot{}, workspacesource.ErrProjectDependencyUnavailable
	}
	return historical.FreezeWithReferences(ctx, projectCopyMediaActor(actor), binding, at, references)
}

func freezeProjectCopyMediaFacts(ctx context.Context, actor identityapp.Principal, owner application.ProjectCopyMediaOwner, binding mediaapp.ProjectCopyBinding, at time.Time, documents []uuid.UUID, facts []mediaapp.ReferenceFact) (mediaapp.ProjectCopySnapshot, error) {
	if len(facts) == 0 {
		return freezeProjectCopyMedia(ctx, actor, owner, binding, at, documents)
	}
	if len(facts) > 4096 || len(documents) > 4096 {
		return mediaapp.ProjectCopySnapshot{}, domain.ErrInvalidProjectCopy
	}
	for _, fact := range facts {
		if err := mediaapp.ValidateCopyReferenceFact(fact); err != nil {
			return mediaapp.ProjectCopySnapshot{}, err
		}
	}
	references, ok := owner.(application.ProjectCopyReferenceMediaOwner)
	if !ok {
		return mediaapp.ProjectCopySnapshot{}, workspacesource.ErrProjectDependencyUnavailable
	}
	return references.FreezeWithReferenceFacts(ctx, projectCopyMediaActor(actor), binding, at, documents, facts)
}

func projectCopyMediaActor(actor identityapp.Principal) mediaapp.Actor {
	return mediaapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}
