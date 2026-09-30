// Package http exposes resource canvas commands without business-generation capabilities.
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// Handler adapts the authorized canvas application service.
type Handler struct{ service *application.Service }

// NewHandler injects the canvas service.
func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

// ListResponse is the document-summary collection contract.
type ListResponse struct {
	Items      []domain.Document `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

// Register installs the scoped endpoints in an authenticated route group.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid/canvases", h.List)
	group.POST("/projects/:pid/canvases", h.Create)
	group.GET("/canvases/:id", h.Get)
	group.PATCH("/canvases/:id", h.Rename)
	group.DELETE("/canvases/:id", h.Delete)
	group.POST("/canvases/:id/commands", h.Commands)
}

// List reads canvas summaries.
// @Summary 项目画布列表
// @ID listCanvases
// @Tags canvases
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {object} ListResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/canvases [get]
func (h *Handler) List(c *gin.Context) {
	id, ok := pathID(c, "pid")
	if !ok {
		return
	}
	docs, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, ListResponse{Items: docs})
}

// Create creates an empty project canvas.
// @Summary 创建画布
// @ID createCanvas
// @Accept json
// @Tags canvases
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-CSRF-Token header string true "CSRF令牌"
// @Param body body application.CreateInput true "画布名称与空scope"
// @Success 201 {object} domain.Document
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/canvases [post]
func (h *Handler) Create(c *gin.Context) {
	id, ok := pathID(c, "pid")
	if !ok {
		return
	}
	var input application.CreateInput
	if !httpapi.Decode(c, &input) {
		return
	}
	doc, err := h.service.Create(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(201, doc)
}

// Get returns the current document graph.
// @Summary 获取画布文档
// @ID getCanvas
// @Tags canvases
// @Produce json
// @Param id path string true "画布UUID"
// @Success 200 {object} domain.Document
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	doc, err := h.service.Get(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, doc)
}

// Commands atomically applies resource, layout, annotation and viewport edits.
// @Summary 提交画布命令批次
// @ID applyCanvasCommands
// @Accept json
// @Tags canvases
// @Produce json
// @Param id path string true "画布UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-CSRF-Token header string true "CSRF令牌"
// @Param body body application.CommandsInput true "乐观版本与命令"
// @Success 200 {object} application.Result
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 413 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id}/commands [post]
func (h *Handler) Commands(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var input application.CommandsInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Execute(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
func pathID(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return uuid.Nil, false
	}
	return id, true
}
func writeError(c *gin.Context, err error) {
	var conflict *application.RevisionConflict
	var commandError *domain.CommandError
	var meta map[string]any
	if errors.As(err, &commandError) {
		meta = map[string]any{"command_index": commandError.Index}
	}
	switch {
	case errors.As(err, &conflict):
		httpapi.WriteProblem(c, http.StatusConflict, "revision_conflict", map[string]any{"current_revision": conflict.CurrentRevision})
	case errors.Is(err, application.ErrNotFound), errors.Is(err, mediaapp.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", meta)
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 422, "idempotency_key_reused", nil)
	case errors.Is(err, domain.ErrUnsupportedCommand):
		httpapi.WriteProblem(c, 422, "unsupported_command", meta)
	case errors.Is(err, domain.ErrInvalidCommand):
		httpapi.WriteProblem(c, 422, "invalid_request", meta)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}

// Rename changes document metadata under an optimistic revision.
// @Summary 修改画布名称
// @ID renameCanvas
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-CSRF-Token header string true "CSRF令牌"
// @Param body body application.RenameInput true "当前修订与名称"
// @Success 200 {object} domain.Document
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id} [patch]
func (h *Handler) Rename(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var input application.RenameInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Rename(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}

// Delete removes the canvas, retaining referenced media facts.
// @Summary 删除画布
// @ID deleteCanvas
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-CSRF-Token header string true "CSRF令牌"
// @Param body body application.DeleteInput true "当前修订"
// @Success 200 {object} application.DeleteResult
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var input application.DeleteInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Delete(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
