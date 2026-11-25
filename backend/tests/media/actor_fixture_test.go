package media_test

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func mediaActor(actor identity.Principal) mediaapp.Actor {
	return mediaapp.Actor{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}

func fixtureMediaActor(c *gin.Context) (mediaapp.Actor, error) {
	value, ok := c.Get("principal")
	if !ok {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	actor, ok := value.(mediaapp.Actor)
	if !ok {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	return actor, nil
}

func mediaIdentityActor(actor mediaapp.Actor) identity.Principal {
	return identity.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch, PlatformEpoch: actor.PlatformEpoch}
}

func fixtureIdentityMediaActor(c *gin.Context) (mediaapp.Actor, error) {
	value, ok := c.Get("principal")
	if !ok {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	actor, ok := value.(identity.Principal)
	if !ok {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	return mediaActor(actor), nil
}

type mediaWorkBoundary struct {
	owner interface {
		HasInflightWork(context.Context, mediaapp.Actor, uuid.UUID) (bool, error)
	}
}

func (g mediaWorkBoundary) HasInflightWork(ctx context.Context, actor identity.Principal, project uuid.UUID) (bool, error) {
	return g.owner.HasInflightWork(ctx, mediaActor(actor), project)
}

func staticMediaActor(*gin.Context) (mediaapp.Actor, error) {
	return mediaapp.Actor{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}, nil
}
