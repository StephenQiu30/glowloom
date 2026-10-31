package http

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AssistantHandler exposes only caller-private conversations of an authorized canvas.
type AssistantHandler struct{ service *application.Assistant }

// NewAssistantHandler injects the private conversation application boundary.
func NewAssistantHandler(service *application.Assistant) *AssistantHandler {
	return &AssistantHandler{service: service}
}

// Register installs scoped, authenticated conversation routes.
func (h *AssistantHandler) Register(group *gin.RouterGroup) {
	group.GET("/canvases/:id/assistant-sessions", h.List)
	group.POST("/canvases/:id/assistant-sessions", h.Create)
	group.GET("/canvases/:id/assistant-sessions/:sid", h.Read)
	group.POST("/canvases/:id/assistant-sessions/:sid/turns", h.Append)
}

// AssistantSessions is a bounded history list.
type AssistantSessions struct {
	Items []application.AssistantSession `json:"items"`
}

// List returns private sessions.
// @Summary 画布助手会话列表
// @ID listCanvasAssistantSessions
// @Tags canvasAssistant
// @Produce json
// @Param id path string true "画布UUID"
// @Success 200 {object} AssistantSessions
// @Router /api/canvases/{id}/assistant-sessions [get]
func (h *AssistantHandler) List(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, AssistantSessions{Items: items})
}

// Create establishes a stable session identity without any model invocation.
// @Summary 新建画布助手会话
// @ID createCanvasAssistantSession
// @Tags canvasAssistant
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param body body application.AssistantCreate true "稳定会话身份"
// @Success 201 {object} application.AssistantSession
// @Router /api/canvases/{id}/assistant-sessions [post]
func (h *AssistantHandler) Create(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in application.AssistantCreate
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Create(c.Request.Context(), identityhttp.Principal(c), id, in)
	if err != nil {
		if errors.Is(err, application.ErrAssistantEmptySession) {
			httpapi.WriteProblem(c, 409, "canvas_assistant_empty_session", nil)
		} else {
			writeError(c, err)
		}
		return
	}
	c.JSON(201, out)
}

// Read resolves a session using both the canvas and caller identity.
// @Summary 读取画布助手会话
// @ID readCanvasAssistantSession
// @Tags canvasAssistant
// @Produce json
// @Param id path string true "画布UUID"
// @Param sid path string true "会话UUID"
// @Success 200 {object} application.AssistantSession
// @Router /api/canvases/{id}/assistant-sessions/{sid} [get]
func (h *AssistantHandler) Read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	sid, ok := httpapi.PathUUID(c, "sid")
	if !ok {
		return
	}
	out, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), id, sid)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, out)
}

// Append binds one private generation input after checking prior turn state.
// @Summary 绑定画布助手下一轮输入
// @ID appendCanvasAssistantTurn
// @Tags canvasAssistant
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param sid path string true "会话UUID"
// @Param body body application.AssistantAppend true "草稿身份和期望修订"
// @Success 200 {object} application.AssistantSession
// @Router /api/canvases/{id}/assistant-sessions/{sid}/turns [post]
func (h *AssistantHandler) Append(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	sid, ok := httpapi.PathUUID(c, "sid")
	if !ok {
		return
	}
	var in application.AssistantAppend
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.Append(c.Request.Context(), identityhttp.Principal(c), id, sid, in)
	if err != nil {
		if errors.Is(err, application.ErrAssistantTurnActive) {
			httpapi.WriteProblem(c, 409, "canvas_assistant_turn_active", nil)
		} else {
			writeError(c, err)
		}
		return
	}
	c.JSON(200, out)
}
