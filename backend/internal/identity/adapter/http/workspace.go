// Package http exposes identity resources and supplies authenticated principals.
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Principal returns the authenticated actor for organization-scoped commands.
func Principal(c *gin.Context) application.Principal {
	value, _ := c.Get("principal")
	principal, _ := value.(application.Principal)
	return principal
}
