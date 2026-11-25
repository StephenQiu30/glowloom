package script_test

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func scriptActor(actor identityapp.Principal) app.Actor { return scriptwire.Actor(actor) }

func scriptIdentityActor(actor app.Actor) identityapp.Principal {
	return scriptwire.IdentityActor(actor)
}

func scriptMediaActor(actor identityapp.Principal) mediaapp.Actor { return mediawire.Actor(actor) }

type scriptWorkGuard func(context.Context, app.Actor, uuid.UUID) (bool, error)

func (g scriptWorkGuard) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	return g(ctx, scriptActor(actor), project)
}

func scriptHTTPActor(c *gin.Context) (app.Actor, error) {
	actor := scriptActor(identityhttp.Principal(c))
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return app.Actor{}, app.ErrSessionInvalid
	}
	return actor, nil
}
