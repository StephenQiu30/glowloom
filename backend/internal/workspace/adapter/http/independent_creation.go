package http

import (
	"context"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// RegisterIndependentCreation exposes the current space's media context.
// @Summary 解析当前空间的独立创作上下文
// @Tags projects
// @ID ensureIndependentCreation
// @Produce json
// @Success 200 {object} ProjectResponse
// @Router /api/creation-context [post]
func RegisterIndependentCreation(group *gin.RouterGroup, ensure func(context.Context, identity.Principal) (domain.Project, error)) {
	group.POST("/creation-context", func(c *gin.Context) {
		project, err := ensure(c.Request.Context(), identityhttp.Principal(c))
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(200, ProjectResponse{
			ID: project.ID, Name: project.Name, Status: project.Status,
			AspectRatio: project.AspectRatio, StyleType: project.StyleType,
			StyleKey: project.StyleKey, Revision: project.Revision, UpdateTime: project.UpdateTime,
			LastModifiedAt: project.CreateTime, LastModifiedBasis: "created",
		})
	})
}
