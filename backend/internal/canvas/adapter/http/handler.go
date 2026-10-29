// Package http exposes authorized canvas snapshots and lifecycle actions.
package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// Handler adapts the authorized canvas application service.
type Handler struct {
	service *application.Service
	notify  func(uuid.UUID)
}

// NewHandler injects the canvas service.
func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

// WithNotification installs a best-effort post-commit live wakeup.
func (h *Handler) WithNotification(notify func(uuid.UUID)) *Handler { h.notify = notify; return h }

// ListResponse is the document-summary collection contract.
type ListResponse struct {
	Items      []application.DocumentDTO `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
}

// Register installs the scoped endpoints in an workspace route group.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid/canvases", h.List)
	group.POST("/projects/:pid/canvases", h.Create)
	group.GET("/canvases/:id", h.Get)
	group.GET("/canvases/:id/director-recovery/:nid", h.ReadDirectorRecovery)
	group.PUT("/canvases/:id/director-recovery/:nid", h.SaveDirectorRecovery)
	group.DELETE("/canvases/:id/director-recovery/:nid", h.DeleteDirectorRecovery)
	group.PATCH("/canvases/:id", h.Rename)
	group.DELETE("/canvases/:id", h.Delete)
}

// List reads canvas summaries.
// @Summary 项目画布列表
// @ID listCanvases
// @Tags canvases
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {object} ListResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/canvases [get]
func (h *Handler) List(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	docs, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	items := make([]application.DocumentDTO, 0, len(docs))
	for _, doc := range docs {
		items = append(items, application.Snapshot(doc))
	}
	c.JSON(200, ListResponse{Items: items})
}

// Create creates an empty project canvas.
// @Summary 创建画布
// @ID createCanvas
// @Accept json
// @Tags canvases
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body application.CreateInput true "画布名称与空scope"
// @Success 201 {object} application.DocumentDTO
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/canvases [post]
func (h *Handler) Create(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "pid")
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
	c.JSON(201, application.Snapshot(doc))
}

// Get returns the current document graph.
// @Summary 获取画布文档
// @ID getCanvas
// @Tags canvases
// @Produce json
// @Param id path string true "画布UUID"
// @Success 200 {object} application.DocumentDTO
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	doc, err := h.service.Get(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, application.Snapshot(doc))
}

func writeError(c *gin.Context, err error) {
	var conflict *application.RevisionConflict
	var operationError *domain.OperationError
	switch {
	case errors.As(err, &operationError):
		status := http.StatusConflict
		if operationError.Code == "canvas_capacity_exceeded" {
			status = http.StatusRequestEntityTooLarge
		}
		httpapi.WriteProblem(c, status, operationError.Code, nil)
	case errors.Is(err, application.ErrMainCanvasManagedByProject):
		httpapi.WriteProblem(c, 409, "main_canvas_managed_by_project", nil)
	case errors.Is(err, application.ErrMainCanvasRequired):
		httpapi.WriteProblem(c, 409, "main_canvas_required", nil)
	case errors.As(err, &conflict):
		httpapi.WriteProblem(c, http.StatusConflict, "revision_conflict", map[string]any{"current_revision": strconv.FormatInt(conflict.CurrentRevision, 10)})
	case errors.Is(err, application.ErrNotFound), errors.Is(err, mediaapp.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, identityapp.ErrSessionInvalid):
		httpapi.WriteProblem(c, http.StatusUnauthorized, "session_invalid", nil)
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_key_reused", nil)
	case errors.Is(err, domain.ErrGraphReadOnly):
		httpapi.WriteProblem(c, 422, "canvas_graph_read_only", nil)
	case errors.Is(err, domain.ErrUnsupportedCommand):
		httpapi.WriteProblem(c, 422, "unsupported_command", nil)
	case errors.Is(err, domain.ErrInvalidCommand):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
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
// @Param body body application.RenameDTO true "当前修订与名称"
// @Success 200 {object} application.DocumentDTO
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id} [patch]
func (h *Handler) Rename(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.RenameDTO
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Rename(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"), application.RenameInput{ExpectedRevision: int64(input.ExpectedRevision), Name: input.Name})
	if err != nil {
		writeError(c, err)
		return
	}
	if h.notify != nil {
		h.notify(id)
	}
	c.JSON(200, application.Snapshot(result))
}

// Delete removes the canvas, retaining referenced media facts.
// @Summary 删除画布
// @ID deleteCanvas
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body application.DeleteDTO true "当前修订"
// @Success 200 {object} application.DeleteResultDTO
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.DeleteDTO
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Delete(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"), application.DeleteInput{ExpectedRevision: int64(input.ExpectedRevision)})
	if err != nil {
		writeError(c, err)
		return
	}
	if h.notify != nil {
		h.notify(id)
	}
	c.JSON(200, application.DeleteResultDTO{ID: result.ID, Revision: application.Revision(result.Revision), Deleted: result.Deleted})
}

// RegisterLocation requires session middleware, but not a selected creation scope.
func (h *Handler) RegisterLocation(group *gin.RouterGroup) {
	group.GET("/canvas-locations/:id", h.Locate)
}

// Locate returns the authorized destination for old and newly shared canvas links.
// @Summary 定位有权访问的画布空间
// @ID locateCanvas
// @Tags canvases
// @Produce json
// @Param id path string true "画布UUID"
// @Success 200 {object} application.Location
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvas-locations/{id} [get]
func (h *Handler) Locate(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	location, err := h.service.Locate(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, location)
}
