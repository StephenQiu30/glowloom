package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// ProjectCreationRecoveryHandler exposes the bounded readonly recovery endpoint.
type ProjectCreationRecoveryHandler struct {
	query *application.ProjectCreationRecoveryQuery
}

// NewProjectCreationRecoveryHandler binds the readonly application query.
func NewProjectCreationRecoveryHandler(query *application.ProjectCreationRecoveryQuery) *ProjectCreationRecoveryHandler {
	return &ProjectCreationRecoveryHandler{query: query}
}

// Register adds the creation-recovery route to authenticated public APIs.
func (h *ProjectCreationRecoveryHandler) Register(group *gin.RouterGroup) {
	group.GET("/project-creations/:key", h.Read)
}

// ProjectCreationRecoveryResponse contains only an authorized current target, with explicit nulls when unrecorded.
type ProjectCreationRecoveryResponse struct {
	State         string     `json:"state" enums:"created,unrecorded"`
	ProjectID     *uuid.UUID `json:"project_id" extensions:"x-nullable"`
	CanvasID      *uuid.UUID `json:"canvas_id" extensions:"x-nullable"`
	ProjectStatus *string    `json:"project_status" extensions:"x-nullable"`
}

// Read resolves a committed creation receipt without sending a creation command.
// @Summary 查询画布创建结果
// @Tags projects
// @ID getProjectCreation
// @Produce json
// @Param key path string true "原创建UUID幂等键"
// @Success 200 {object} ProjectCreationRecoveryResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 410 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-creations/{key} [get]
func (h *ProjectCreationRecoveryHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, err := uuid.Parse(c.Param("key"))
	if err != nil || key == uuid.Nil || key.String() != c.Param("key") {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	result, err := h.query.Resolve(c.Request.Context(), identityhttp.Principal(c), key)
	switch {
	case errors.Is(err, application.ErrCreationTargetUnavailable):
		httpapi.WriteProblem(c, 410, "creation_target_unavailable", nil)
	case errors.Is(err, application.ErrMainCanvasUnavailable):
		httpapi.WriteProblem(c, 409, "main_canvas_unavailable", nil)
	case err != nil:
		writeError(c, err)
	default:
		c.JSON(200, ProjectCreationRecoveryResponse{State: result.State, ProjectID: result.ProjectID, CanvasID: result.CanvasID, ProjectStatus: result.ProjectStatus})
	}
}
