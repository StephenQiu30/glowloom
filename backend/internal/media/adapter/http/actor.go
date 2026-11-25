package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// ActorResolver reads only the server-authenticated account/session reference.
// Media still resolves current resource authority inside its actual owner ports.
type ActorResolver func(*gin.Context) (application.Actor, error)

func requestActor(c *gin.Context, resolve ActorResolver) (application.Actor, bool) {
	if c == nil {
		return application.Actor{}, false
	}
	if resolve == nil {
		writeError(c, application.ErrSessionInvalid)
		return application.Actor{}, false
	}
	actor, err := resolve(c)
	if err == nil && (actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1) {
		err = application.ErrSessionInvalid
	}
	if err != nil {
		writeError(c, err)
		return application.Actor{}, false
	}
	return actor, true
}
