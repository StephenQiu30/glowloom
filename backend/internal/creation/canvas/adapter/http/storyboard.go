package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// StoryboardHandler adapts the authoritative canvas storyboard commands.
type StoryboardHandler struct {
	service *application.YjsService
	notify  func(uuid.UUID)
}

// NewStoryboardHandler binds storyboards to the existing canvas runtime.
func NewStoryboardHandler(service *application.YjsService, notify func(uuid.UUID)) *StoryboardHandler {
	return &StoryboardHandler{service: service, notify: notify}
}

// Register installs the storyboard's authorized read and write entries.
func (h *StoryboardHandler) Register(group *gin.RouterGroup) {
	group.GET("/canvases/:id/storyboard", h.Get)
	group.POST("/canvases/:id/storyboard", h.Write)
}

// Get returns the current ordered shots at the canvas revision.
// @Summary 普通画布故事板
// @ID getCanvasStoryboard
// @Tags canvases
// @Produce json
// @Param id path string true "画布 UUID"
// @Success 200 {object} application.StoryboardView
// @Failure 404 {object} httpapi.Problem
// @Router /api/canvases/{id}/storyboard [get]
func (h *StoryboardHandler) Get(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	view, err := h.service.ReadStoryboard(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// Write validates and applies one storyboard edit or first publication.
// @Summary 正式写入普通画布故事板
// @ID writeCanvasStoryboard
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布 UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body domain.StoryboardCommand true "镜头操作"
// @Success 200 {object} application.StoryboardView
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvases/{id}/storyboard [post]
func (h *StoryboardHandler) Write(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		writeError(c, domain.ErrInvalidCommand)
		return
	}
	var input domain.StoryboardCommand
	if !httpapi.Decode(c, &input) {
		return
	}
	ctx, actor := c.Request.Context(), identityhttp.Principal(c)
	prepared, err := h.service.PrepareStoryboard(ctx, actor, id, key, input)
	if err != nil {
		WriteStoryboardError(c, err)
		return
	}
	view, err := h.service.CommitStoryboard(ctx, actor, prepared)
	if err != nil {
		WriteStoryboardError(c, err)
		return
	}
	if h.notify != nil && prepared.Command != nil {
		h.notify(id)
	}
	c.JSON(http.StatusOK, view)
}

// WriteStoryboardError maps owning canvas failures at the composed public routes.
func WriteStoryboardError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrStoryboardConflict) || errors.Is(err, application.ErrYjsContention) {
		httpapi.WriteProblem(c, http.StatusConflict, "storyboard_revision_conflict", nil)
		return
	}
	writeError(c, err)
}
