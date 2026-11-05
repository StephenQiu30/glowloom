package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
)

// RunPage is one cursor page of public run summaries.
type RunPage = runtimeclient.RunPage

// AcceptedRun is the durable response to an accepted creative input.
type AcceptedRun = runtimeclient.AcceptedRun

// CurrentSubmission exposes the current accepted input and unresolved attempts.
type CurrentSubmission = runtimeclient.CurrentSubmission

// MessagePage is one cursor page of committed public messages.
type MessagePage = runtimeclient.MessagePage

// PublicMessage is an authorized logical message exposed by the runtime.
type PublicMessage = runtimeclient.PublicMessage

// WithControlStatus injects the reader for durable control and delivery receipts.
func (h *PublicHandler) WithControlStatus(reader app.ControlStatusReader) *PublicHandler {
	h.controls = reader
	return h
}

// WithConversationInputs injects current draft, source, quote and model authorization.
func (h *PublicHandler) WithConversationInputs(inputs app.ConversationInputFreezer) *PublicHandler {
	h.inputs = inputs
	return h
}

// WithConversationHistory injects current source checks for historical conversation reads.
func (h *PublicHandler) WithConversationHistory(history app.ConversationHistoryReader, unresolved app.UnresolvedConversationReader) *PublicHandler {
	h.history = history
	h.unresolved = unresolved
	return h
}

// ControlStatus reads Go's durable decision even while Agent is unavailable.
// @Summary 读取 Agent 控制决定与投递回执
// @ID getAgentControlStatus
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param rid path string true "运行UUID"
// @Param request_id query string false "原控制请求UUID"
// @Success 200 {object} app.ControlStatus
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs/{rid}/commands/current [get]
func (h *PublicHandler) ControlStatus(c *gin.Context) {
	a, ok := h.authority(c)
	if !ok {
		return
	}
	var requestID *uuid.UUID
	if value := c.Query("request_id"); value != "" {
		parsed, err := uuid.Parse(value)
		if err != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		requestID = &parsed
	}
	if h.controls == nil {
		if a.Current != nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		send(c, app.ControlStatus{AuthorizationRevision: a.AuthorizationRevision}, nil)
		return
	}
	out, err := h.controls.ReadControlStatus(c.Request.Context(), identityhttp.Principal(c), a.Scope.ProjectID, a.SessionID, a.RootRunID, requestID)
	send(c, out, err)
}

// Runs lists bounded history without exposing another creator's sessions.
// @Summary 分页读取 Agent 会话全部尝试摘要
// @ID listAgentRuns
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param limit query int false "页大小，默认20，最大50"
// @Param cursor query string false "历史游标"
// @Success 200 {object} runtimeclient.RunPage
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs [get]
func (h *PublicHandler) Runs(c *gin.Context) {
	s, ok := h.scope(c, false)
	if ok {
		h.proxy(c, "listAgentRuns", s, nil)
	}
}

// Submission recovers the original durable admission.
// @Summary 找回 Agent 原输入受理回执
// @ID getAgentSubmission
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param requestId path string true "原请求UUID"
// @Success 200 {object} runtimeclient.AcceptedRun
// @Router /api/projects/{pid}/agent/sessions/{sid}/submissions/{requestId} [get]
func (h *PublicHandler) Submission(c *gin.Context) {
	s, ok := h.scope(c, false)
	if ok {
		h.proxy(c, "getAgentSubmission", s, nil)
	}
}

// CurrentSubmission discovers accepted input after page-memory recovery keys were lost.
// @Summary 发现 Agent 当前已受理输入
// @ID getCurrentAgentSubmission
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Success 200 {object} runtimeclient.CurrentSubmission
// @Router /api/projects/{pid}/agent/sessions/{sid}/submissions/current [get]
func (h *PublicHandler) CurrentSubmission(c *gin.Context) {
	s, ok := h.scope(c, false)
	if ok {
		h.proxy(c, "getCurrentAgentSubmission", s, nil)
	}
}

// Messages reads an older page at a fixed committed watermark.
// @Summary 分页读取 Agent 公开消息
// @ID listAgentMessages
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param rid path string true "运行UUID"
// @Param limit query int false "页大小，默认50，最大100"
// @Param before query string false "消息游标"
// @Param through query string true "快照截止游标"
// @Success 200 {object} runtimeclient.MessagePage
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs/{rid}/messages [get]
func (h *PublicHandler) Messages(c *gin.Context) {
	a, ok := h.authority(c)
	if ok {
		h.proxy(c, "listAgentMessages", a.Scope, nil)
	}
}

// Message returns a whole authorized logical message for quoting.
// @Summary 读取 Agent 公开消息引用原文
// @ID getAgentMessage
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param rid path string true "运行UUID"
// @Param messageId path string true "消息身份"
// @Success 200 {object} runtimeclient.PublicMessage
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs/{rid}/messages/{messageId} [get]
func (h *PublicHandler) Message(c *gin.Context) {
	a, ok := h.authority(c)
	if ok {
		h.proxy(c, "getAgentMessage", a.Scope, nil)
	}
}
