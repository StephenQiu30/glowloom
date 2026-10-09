// Package http exposes identity resources and supplies authenticated principals.
package http

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// WorkspaceProvider resolves the durable identity behind the current workspace.
type WorkspaceProvider interface {
	EnsureWorkspace(context.Context) (domain.User, error)
}

// Workspace supplies one identity without login, browser cookies or session state.
func Workspace(store WorkspaceProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := store.EnsureWorkspace(c.Request.Context())
		if errors.Is(err, application.ErrForbidden) {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			return
		}
		if err != nil {
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
			return
		}
		if user.ID == uuid.Nil || user.OrgID == uuid.Nil || user.Status != domain.StatusActive ||
			user.MustChangePassword || user.Role != domain.RoleProducer {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			return
		}
		c.Set("principal", application.Principal{ID: user.ID, OrgID: user.OrgID,
			LoginName: user.LoginName, DisplayName: user.DisplayName, Role: user.Role})
		c.Next()
	}
}

// Principal returns the authenticated actor for organization-scoped commands.
func Principal(c *gin.Context) application.Principal {
	value, _ := c.Get("principal")
	principal, _ := value.(application.Principal)
	return principal
}
