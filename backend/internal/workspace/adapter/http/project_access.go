package http

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// ProjectAccessStore checks current ownership before entering project routes.
type ProjectAccessStore interface {
	RequireOwnedProject(context.Context, identityapp.Principal, uuid.UUID) error
}

// RequireProjectAccess follows session authentication and guards explicit project scopes.
func RequireProjectAccess(store ProjectAccessStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids := []string{c.Param("pid"), c.Param("project_id"), c.Query("project_id")}
		for _, raw := range ids {
			if raw == "" {
				continue
			}
			id, err := uuid.Parse(raw)
			if err != nil || id == uuid.Nil {
				httpapi.WriteProblem(c, 422, "invalid_request", nil)
				c.Abort()
				return
			}
			if err := store.RequireOwnedProject(c.Request.Context(), identityhttp.Principal(c), id); err != nil {
				writeError(c, err)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
