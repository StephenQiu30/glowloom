package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// RequireCreationScope binds only content routes to an explicit requested scope.
// Consumers must repeat the exact session/scope check in their own transaction.
func RequireCreationScope(scopes application.RequestScopeResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("X-Creation-Scope")
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			httpapi.WriteProblem(c, 422, "invalid_creation_scope", nil)
			c.Abort()
			return
		}
		if scopes == nil {
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
			c.Abort()
			return
		}
		actor := identityhttp.Principal(c)
		write := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions
		scope, err := scopes.Resolve(c.Request.Context(), actor, id, write)
		if err != nil {
			writeError(c, err)
			c.Abort()
			return
		}
		if scope.ID != id || scope.PhysicalScopeID == uuid.Nil {
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
			c.Abort()
			return
		}
		actor.ScopeID, actor.OrgID = scope.ID, scope.PhysicalScopeID
		c.Set("principal", actor)
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}
}
