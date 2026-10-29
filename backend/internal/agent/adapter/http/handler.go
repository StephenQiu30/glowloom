// Package http exposes project-private, confirmed Agent orchestration.
package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// Handler serves account-private Agent session endpoints.
type Handler struct{ service *application.Service }

// NewHandler binds the session service.
func NewHandler(s *application.Service) *Handler { return &Handler{service: s} }

// Register installs authenticated project routes.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/agent-sessions", h.List)
	g.POST("/projects/:pid/agent-sessions", h.Create)
	g.GET("/projects/:pid/agent-sessions/:sid", h.Read)
	g.POST("/projects/:pid/agent-sessions/:sid/turns", h.Start)
	g.POST("/projects/:pid/agent-sessions/:sid/advance", h.Advance)
	g.POST("/projects/:pid/agent-sessions/:sid/confirm", h.Confirm)
	g.POST("/projects/:pid/agent-sessions/:sid/stop", h.Stop)
}

func send(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(200, value)
		return
	}
	status, code := 503, "agent_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, op.ErrInvalidFreeQuote), errors.Is(err, op.ErrFreeQuoteInputNotReady):
		status, code = 422, "invalid_agent_request"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, op.ErrQuoteStale):
		status, code = 409, "agent_revision_conflict"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "agent_session_not_found"
	case errors.Is(err, identity.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, op.ErrFreeQuoteModelMissing):
		status, code = 409, "agent_model_unavailable"
	}
	httpapi.WriteProblem(c, status, code, nil)
}

// Create creates a private project conversation, without calling a model.
// @Summary 新建项目 Agent 私有会话
// @ID createAgentSession
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param body body CreateRequest true "会话"
// @Success 200 {object} domain.Session
// @Router /api/projects/{pid}/agent-sessions [post]
func (h *Handler) Create(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var in CreateRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if !application.ValidTitle(in.Title) {
		send(c, nil, domain.ErrInvalid)
		return
	}
	v, e := h.service.Store.Create(c.Request.Context(), identityhttp.Principal(c), pid, in.RequestID, in.Title)
	send(c, v, e)
}

// CreateRequest identifies an idempotent new conversation.
type CreateRequest struct {
	RequestID uuid.UUID `json:"request_id"`
	Title     string    `json:"title"`
}

// List reads only the current actor's project sessions.
// @Summary 当前项目我的 Agent 会话
// @ID listAgentSessions
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {array} domain.Session
// @Router /api/projects/{pid}/agent-sessions [get]
func (h *Handler) List(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	v, e := h.service.Store.List(c.Request.Context(), identityhttp.Principal(c), pid)
	send(c, v, e)
}

func ids(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	sid, ok := httpapi.PathUUID(c, "sid")
	return pid, sid, ok
}

// Read is deliberately side-effect free.
// @Summary 读取 Agent 会话与阶段回执
// @ID getAgentSession
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Success 200 {object} domain.Session
// @Router /api/projects/{pid}/agent-sessions/{sid} [get]
func (h *Handler) Read(c *gin.Context) {
	pid, sid, ok := ids(c)
	if !ok {
		return
	}
	v, e := h.service.Store.Read(c.Request.Context(), identityhttp.Principal(c), pid, sid)
	send(c, v, e)
}

// Start persists a new turn before requesting a quote.
// @Summary 开始一轮 Agent 图片创作
// @ID startAgentTurn
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param body body domain.TurnInput true "输入与模型选择"
// @Success 200 {object} domain.Session
// @Router /api/projects/{pid}/agent-sessions/{sid}/turns [post]
func (h *Handler) Start(c *gin.Context) {
	pid, sid, ok := ids(c)
	if !ok {
		return
	}
	var in domain.TurnInput
	if !httpapi.Decode(c, &in) {
		return
	}
	v, e := h.service.Start(c.Request.Context(), identityhttp.Principal(c), pid, sid, in)
	send(c, v, e)
}

// Advance uses durable receipts and never authorizes spending.
// @Summary 恢复 Agent 轮次并准备下一阶段报价
// @ID advanceAgentSession
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Success 200 {object} domain.Session
// @Router /api/projects/{pid}/agent-sessions/{sid}/advance [post]
func (h *Handler) Advance(c *gin.Context) {
	pid, sid, ok := ids(c)
	if !ok {
		return
	}
	v, e := h.service.Advance(c.Request.Context(), identityhttp.Principal(c), pid, sid)
	send(c, v, e)
}

// Confirm authorizes exactly the displayed phase operation.
// @Summary 确认 Agent 当前阶段积分
// @ID confirmAgentStage
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param body body application.ConfirmInput true "阶段与修订"
// @Success 200 {object} domain.Session
// @Router /api/projects/{pid}/agent-sessions/{sid}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	pid, sid, ok := ids(c)
	if !ok {
		return
	}
	var in application.ConfirmInput
	if !httpapi.Decode(c, &in) {
		return
	}
	v, e := h.service.Confirm(c.Request.Context(), identityhttp.Principal(c), pid, sid, in)
	send(c, v, e)
}

// Stop prevents the next stage; it does not promise supplier cancellation.
// @Summary 停止 Agent 后续阶段
// @ID stopAgentSession
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param body body StopRequest true "当前修订"
// @Success 200 {object} domain.Session
// @Router /api/projects/{pid}/agent-sessions/{sid}/stop [post]
func (h *Handler) Stop(c *gin.Context) {
	pid, sid, ok := ids(c)
	if !ok {
		return
	}
	var in StopRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	v, e := h.service.Stop(c.Request.Context(), identityhttp.Principal(c), pid, sid, in.ExpectedRevision)
	send(c, v, e)
}

// StopRequest guards a stop against a newer session revision.
type StopRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}
