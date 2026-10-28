// Package http exposes current-space subject settings through protected REST routes.
package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// Handler requires the existing session, Origin, CSRF and creation-scope middleware.
type Handler struct {
	commands *application.Service
	query    application.Query
}

// NewHandler injects explicit command and read boundaries.
func NewHandler(commands *application.Service, query application.Query) *Handler {
	return &Handler{commands: commands, query: query}
}

// Register installs only authenticated current-space subject routes.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/subjects", h.List)
	group.GET("/subject-commands/:key", h.Recover)
	group.POST("/subjects", h.Create)
	group.GET("/subjects/:id", h.Get)
	group.PUT("/subjects/:id", h.Update)
	group.DELETE("/subjects/:id", h.Delete)
	group.PUT("/subjects/:id/favorite", h.Favorite)
	group.POST("/subjects/batch", h.Batch)
	group.GET("/subjects/:id/versions/:version", h.Version)
}

func pathID(c *gin.Context, key string) (uuid.UUID, bool) { return httpapi.PathUUID(c, key) }
func commandKey(c *gin.Context) (uuid.UUID, bool) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return uuid.Nil, false
	}
	return key, true
}
func currentActor(c *gin.Context) identity.Principal { return identityhttp.Principal(c) }
func writeError(c *gin.Context, err error) {
	status, code := 503, "dependency_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 422, "invalid_request"
	case errors.Is(err, application.ErrNotFound), errors.Is(err, workspace.ErrScopeNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, application.ErrConflict):
		status, code = 409, "revision_conflict"
	case errors.Is(err, application.ErrKeyConflict):
		status, code = 409, "idempotency_conflict"
	case errors.Is(err, identity.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, identity.ErrSessionInvalid):
		status, code = 401, "unauthorized"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
