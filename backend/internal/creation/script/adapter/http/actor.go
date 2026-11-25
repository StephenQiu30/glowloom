package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
)

// ActorResolver projects authenticated identity at the composition boundary.
type ActorResolver func(*gin.Context) (application.Actor, error)

func resolveActor(c *gin.Context, resolve ActorResolver) (application.Actor, error) {
	if resolve == nil {
		return application.Actor{}, application.ErrSessionInvalid
	}
	actor, err := resolve(c)
	if err != nil {
		return application.Actor{}, err
	}
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return application.Actor{}, application.ErrSessionInvalid
	}
	return actor, nil
}
