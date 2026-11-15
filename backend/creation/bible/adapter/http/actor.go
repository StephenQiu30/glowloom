package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"
)

// ActorResolver reads an authenticated server-side reference, never public actor
// headers. Current content authorization remains inside the store transaction.
type ActorResolver func(*gin.Context) (application.Actor, error)

func (h *Handler) actor(c *gin.Context) (application.Actor, bool) {
	if h == nil || h.resolve == nil || c == nil {
		if c != nil {
			writeError(c, application.ErrSessionInvalid)
		}
		return application.Actor{}, false
	}
	actor, err := h.resolve(c)
	if err == nil && (actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1) {
		err = application.ErrSessionInvalid
	}
	if err != nil {
		writeError(c, err)
		return application.Actor{}, false
	}
	return actor, true
}
