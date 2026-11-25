package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// RegisterProjectMemory exposes human confirmation and project history.
func RegisterProjectMemory(group *gin.RouterGroup, service *application.ProjectMemories) {
	h := projectMemoryHandler{service: service}
	group.GET("/projects/:pid/memory", h.list)
	group.GET("/projects/:pid/memory/:key", h.history)
	group.PUT("/projects/:pid/memory/:key", h.change)
}

type projectMemoryHandler struct{ service *application.ProjectMemories }

// @Summary 当前项目约定
// @Tags projectMemory
// @ID listProjectMemory
// @Produce json
// @Param pid path string true "项目 UUID"
// @Param shot_id query string false "镜头 UUID"
// @Success 200 {object} application.ProjectMemoryView
// @Failure 404 {object} httpapi.Problem
// @Router /api/projects/{pid}/memory [get]
func (h projectMemoryHandler) list(c *gin.Context) {
	id, err := uuid.Parse(c.Param("pid"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	shot, ok := projectMemoryShot(c)
	if !ok {
		return
	}
	view, err := h.service.ListForShot(c.Request.Context(), identityhttp.Principal(c), id, shot)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	c.JSON(200, view)
}

// @Summary 项目约定的修订记录
// @Tags projectMemory
// @ID listProjectMemoryHistory
// @Produce json
// @Param pid path string true "项目 UUID"
// @Param key path string true "约定名称"
// @Param before query integer false "修订号上界"
// @Param shot_id query string false "镜头 UUID"
// @Success 200 {array} application.ProjectMemory
// @Failure 404 {object} httpapi.Problem
// @Router /api/projects/{pid}/memory/{key} [get]
func (h projectMemoryHandler) history(c *gin.Context) {
	id, err := uuid.Parse(c.Param("pid"))
	before, cursorErr := strconv.ParseInt(c.DefaultQuery("before", "0"), 10, 64)
	if err != nil || id == uuid.Nil || cursorErr != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	shot, ok := projectMemoryShot(c)
	if !ok {
		return
	}
	view, err := h.service.HistoryForShot(c.Request.Context(), identityhttp.Principal(c), id, shot, c.Param("key"), before)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	c.JSON(200, view)
}

// @Summary 确认发布、更正或撤回项目约定
// @Tags projectMemory
// @ID publishProjectMemory
// @Accept json
// @Produce json
// @Param pid path string true "项目 UUID"
// @Param key path string true "约定名称"
// @Param Idempotency-Key header string true "动作 UUID"
// @Param body body application.ProjectMemoryChange true "明确确认的约定内容"
// @Success 200 {object} application.ProjectMemory
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/memory/{key} [put]
func (h projectMemoryHandler) change(c *gin.Context) {
	id, err := uuid.Parse(c.Param("pid"))
	request, keyErr := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || id == uuid.Nil || keyErr != nil || request == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	if err != nil || !utf8.Valid(raw) {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var input application.ProjectMemoryChange
	if dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	revision, err := h.service.Change(c.Request.Context(), identityhttp.Principal(c), id, c.Param("key"), request, input)
	if err != nil {
		writeMemoryError(c, err)
		return
	}
	c.JSON(200, revision)
}

func writeMemoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidProjectMemory):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, application.ErrProjectMemoryLimit):
		httpapi.WriteProblem(c, 409, "project_memory_limit", nil)
	default:
		writeLifecycleError(c, err)
	}
}

func projectMemoryShot(c *gin.Context) (*uuid.UUID, bool) {
	raw, present := c.GetQuery("shot_id")
	if !present {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return nil, false
	}
	return &id, true
}
