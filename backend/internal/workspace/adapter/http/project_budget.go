package http

import (
	"github.com/gin-gonic/gin"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// ProjectBudgetHandler exposes an authorized budget snapshot for cost confirmation.
type ProjectBudgetHandler struct {
	query *application.ProjectBudgetQuery
}

// NewProjectBudgetHandler injects the project-scoped budget query.
func NewProjectBudgetHandler(query *application.ProjectBudgetQuery) *ProjectBudgetHandler {
	return &ProjectBudgetHandler{query: query}
}

// Register mounts a read-only project budget endpoint.
func (h *ProjectBudgetHandler) Register(g *gin.RouterGroup) { g.GET("/projects/:pid/budget", h.Read) }

// Read returns a fresh snapshot; confirmation still rechecks and reserves funds.
// @Summary 项目预算余额
// @ID getProjectBudget
// @Tags projects
// @Produce json
// @Param pid path string true "项目ID"
// @Success 200 {object} application.ProjectBudgetView
// @Router /api/projects/{pid}/budget [get]
func (h *ProjectBudgetHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	result, err := h.query.Read(c.Request.Context(), identity.Principal(c), id)
	if err != nil {
		writeLifecycleError(c, err)
		return
	}
	c.JSON(200, result)
}
