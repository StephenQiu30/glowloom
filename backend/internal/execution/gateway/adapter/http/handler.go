// Package http exposes native SDK protocols and the rebuilt AI management boundary.
package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Sealer is a public-key-only write dependency; no HTTP response exposes a provider secret.
type Sealer interface {
	Seal(uuid.UUID, uuid.UUID, json.RawMessage) ([]byte, error)
	KeyID() string
}

// Handler exposes native protocol ingress and administrative configuration.
type Handler struct {
	db       *gorm.DB
	auth     pg.Authority
	provider application.Provider
	sealer   Sealer
}

// New binds transport, authority and credential encryption to the gateway.
func New(db *gorm.DB, auth pg.Authority, transport application.Provider, sealer Sealer) *Handler {
	return &Handler{db, auth, transport, sealer}
}
func (h *Handler) store(a identity.Principal) *pg.Store { return pg.New(h.db, a, h.auth) }

// Register installs SDK endpoints with scoped integration grants, plus session management.
func (h *Handler) Register(api, session *gin.RouterGroup) {
	sdk := api.Group("/ai/v1", h.integration)
	sdk.GET("/models", h.models)
	sdk.POST("/chat/completions", h.chat)
	sdk.POST("/responses", h.responses)
	sdk.POST("/messages", h.messages)
	session.POST("/ai/grants", h.grant)
	session.DELETE("/ai/grants/:id", h.revokeGrant)
	session.GET("/ai/wallet", h.wallet)
	session.GET("/ai/bills", h.bills)
	session.GET("/ai/requests/:id", h.read)
	session.GET("/ai/admin/configuration", h.configuration)
	session.PUT("/ai/admin/budgets", h.budget)
	session.POST("/ai/admin/models", h.model)
	session.POST("/ai/admin/routes", h.route)
	session.POST("/ai/admin/routes/delete", h.deleteRoutes)
	session.GET("/ai/admin/onboarding/:id", h.onboarding)
	session.POST("/ai/admin/onboarding/:id/verify", h.verifyOnboarding)
	session.POST("/ai/admin/onboarding/:id/rollback", h.rollbackOnboarding)
	session.POST("/ai/admin/credits", h.credits)
	session.POST("/ai/admin/reconciliations", h.reconcile)
	session.PUT("/ai/admin/credentials/:id", h.credential)
}
func (h *Handler) integration(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if token == "" {
		token = c.GetHeader("X-Api-Key")
	}
	grant, err := pg.ResolveGrant(c.Request.Context(), h.db, token)
	if err != nil {
		c.Header("x-should-retry", "false")
		c.JSON(401, gin.H{"type": "error", "code": "integration_grant_invalid", "error": gin.H{"type": "authentication_error", "message": "integration_grant_invalid"}})
		c.Abort()
		return
	}
	c.Set("ai_grant", grant)
	c.Next()
}
func (h *Handler) native(protocol string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Anthropic-Beta") != "" || c.GetHeader("OpenAI-Beta") != "" {
			problem(c, domain.ErrInvalid)
			return
		}
		grant, ok := c.MustGet("ai_grant").(pg.Integration)
		if !ok {
			problem(c, domain.ErrForbidden)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
		if err != nil {
			problem(c, domain.ErrInvalid)
			return
		}
		var envelope struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			problem(c, domain.ErrInvalid)
			return
		}
		store := h.store(grant.Actor).WithGrant(grant.ID)
		model, err := store.CallModel(c.Request.Context(), grant.Scope, grant.ID, c.GetHeader("Idempotency-Key"), "sdk", envelope.Model, protocol)
		if err != nil {
			problem(c, err)
			return
		}
		call, err := application.Prepare(model, grant.Actor.ID, grant.Scope, uuid.Nil, grant.ID, c.GetHeader("Idempotency-Key"), "sdk", raw)
		if err != nil {
			problem(c, err)
			return
		}
		call.ID = uuid.NewSHA1(grant.ID, []byte("sdk/"+call.Key))
		wrote := false
		forwarded := false
		emit := func(raw json.RawMessage) error {
			var event provider.Event
			if json.Unmarshal(raw, &event) != nil {
				return domain.ErrUnknown
			}
			if !wrote {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-store")
				c.Header("X-Accel-Buffering", "no")
				c.Header("X-Glowloom-Request-Id", call.ID.String())
				wrote = true
			}
			if event.Event != "" {
				if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event.Event); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", event.Data); err != nil {
				return err
			}
			c.Writer.Flush()
			return c.Request.Context().Err()
		}
		record, err := application.New(store, h.provider).Execute(c.Request.Context(), call, func(raw json.RawMessage) error { forwarded = true; return emit(raw) })
		if err != nil {
			if wrote {
				_ = emit(json.RawMessage(`{"event":"error","data":"{\"type\":\"error\",\"error\":{\"type\":\"ai_result_unknown\",\"message\":\"query the original request\"}}"}`))
			} else {
				c.Header("X-Glowloom-Request-Id", record.ID.String())
				problem(c, err)
			}
			return
		}
		call.ID = record.ID
		c.Header("X-Glowloom-Request-Id", record.ID.String())
		if record.State != "settled" || record.Receipt == nil {
			problem(c, domain.ErrUnknown)
			return
		}
		if call.Stream {
			for _, frame := range record.Receipt.Events {
				if !forwarded || provider.Terminal(frame) {
					if err := emit(frame); err != nil {
						return
					}
				}
			}
			return
		}
		c.Data(http.StatusOK, "application/json", record.Receipt.Body)
	}
}
func scope(c *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.GetHeader("X-Creation-Scope"))
	if err != nil {
		id = identityhttp.Principal(c).ScopeID
		err = nil
	}
	if err != nil || id == uuid.Nil {
		return id, domain.ErrInvalid
	}
	return id, nil
}

// grant 签发限定模型和预算的调用凭据.
// @Summary 签发限定模型和预算的调用凭据
// @ID issueAiIntegrationGrant
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 201
// @Router /api/ai/grants [post]
func (h *Handler) grant(c *gin.Context) {
	id, err := scope(c)
	if err != nil {
		problem(c, err)
		return
	}
	var in struct {
		Models  []uuid.UUID `json:"models"`
		Budget  int64       `json:"budget"`
		Seconds int         `json:"seconds"`
	}
	if c.ShouldBindJSON(&in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	token, err := h.store(identityhttp.Principal(c)).IssueGrant(c.Request.Context(), id, in.Models, in.Budget, in.Seconds)
	if err != nil {
		problem(c, err)
		return
	}
	issued, e := pg.ResolveGrant(c.Request.Context(), h.db, token)
	if e != nil {
		problem(c, e)
		return
	}
	c.JSON(201, gin.H{"id": issued.ID, "token": token, "expires_in": in.Seconds})
}

// wallet 读取统一积分余额.
// @Summary 读取统一积分余额
// @ID readAiWallet
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param X-Scope-ID header string false "创作空间"
// @Success 200 {object} object
// @Router /api/ai/wallet [get]
func (h *Handler) wallet(c *gin.Context) {
	id, err := scope(c)
	if err != nil {
		problem(c, err)
		return
	}
	out, err := h.store(identityhttp.Principal(c)).Wallet(c.Request.Context(), id)
	if err != nil {
		problem(c, err)
		return
	}
	c.JSON(200, out)
}

// bills 读取统一模型账单.
// @Summary 读取统一模型账单
// @ID listAiBills
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param X-Scope-ID header string false "创作空间"
// @Success 200 {object} object
// @Router /api/ai/bills [get]
func (h *Handler) bills(c *gin.Context) {
	id, err := scope(c)
	if err != nil {
		problem(c, err)
		return
	}
	out, err := h.store(identityhttp.Principal(c)).Bills(c.Request.Context(), id)
	if err != nil {
		problem(c, err)
		return
	}
	for i := range out {
		out[i].Receipt = nil
	}
	c.JSON(200, out)
}

// read 读取原调用和结算事实.
// @Summary 读取原调用和结算事实
// @ID readAiRequest
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param id path string true "标识"
// @Param X-Scope-ID header string false "创作空间"
// @Success 200 {object} object
// @Router /api/ai/requests/{id} [get]
func (h *Handler) read(c *gin.Context) {
	id, err := scope(c)
	rid, e := uuid.Parse(c.Param("id"))
	if err != nil || e != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	out, err := h.store(identityhttp.Principal(c)).Read(c.Request.Context(), id, rid)
	if err != nil {
		problem(c, err)
		return
	}
	if out.Receipt != nil {
		out.Receipt.PurchaseMicros = 0
	}
	c.JSON(200, out)
}

// credits 按业务凭证发放积分.
// @Summary 按业务凭证发放积分
// @ID grantAiCredits
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 201
// @Router /api/ai/admin/credits [post]
func (h *Handler) credits(c *gin.Context) {
	var in struct {
		Scope   uuid.UUID `json:"scope_id"`
		Key     string    `json:"key"`
		Credits int64     `json:"credits"`
	}
	if c.ShouldBindJSON(&in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	if err := h.store(identityhttp.Principal(c)).Grant(c.Request.Context(), in.Scope, in.Key, in.Credits); err != nil {
		problem(c, err)
		return
	}
	c.Status(201)
}

// credential 调整账号可用状态.
// @Summary 调整账号可用状态
// @ID changeAiCredentialState
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param id path string true "标识"
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 204
// @Router /api/ai/admin/credentials/{id} [put]
func (h *Handler) credential(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	var in struct {
		State string `json:"state"`
	}
	if err != nil || c.ShouldBindJSON(&in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	if err = h.store(identityhttp.Principal(c)).CredentialState(c.Request.Context(), id, in.State); err != nil {
		problem(c, err)
		return
	}
	c.Status(204)
}
func problem(c *gin.Context, err error) {
	c.Header("x-should-retry", "false")
	status, kind := 500, "ai_internal"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, kind = 400, "invalid_request_error"
	case errors.Is(err, domain.ErrForbidden):
		status, kind = 403, "permission_error"
	case errors.Is(err, domain.ErrConflict):
		status, kind = 409, "idempotency_conflict"
	case errors.Is(err, domain.ErrFunds):
		status, kind = 402, "insufficient_credits"
	case errors.Is(err, domain.ErrBudget):
		status, kind = 402, "budget_exceeded"
	case errors.Is(err, domain.ErrUnavailable):
		status, kind = 503, "model_unavailable"
	case errors.Is(err, domain.ErrUnknown):
		status, kind = 409, "ai_result_unknown"
	}
	c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": kind, "message": kind}})
}

// models 列出调用凭据允许的原生协议模型.
// @Summary 列出调用凭据允许的原生协议模型
// @ID listAiIntegrationModels
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Success 200 {object} object
// @Router /api/ai/v1/models [get]
func (h *Handler) models(c *gin.Context) {
	grant := c.MustGet("ai_grant").(pg.Integration)
	models, err := h.store(grant.Actor).WithGrant(grant.ID).GrantModels(c.Request.Context(), grant.Scope)
	if err != nil {
		problem(c, err)
		return
	}
	rows := []gin.H{}
	for _, m := range models {
		rows = append(rows, gin.H{"id": m.Name, "object": "model", "created": 0, "owned_by": "glowloom", "protocol": m.Protocol, "tools": m.Tools, "stream": m.Stream})
	}
	c.JSON(200, gin.H{"object": "list", "data": rows})
}

// revokeGrant 撤销调用凭据.
// @Summary 撤销调用凭据
// @ID revokeAiIntegrationGrant
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param id path string true "标识"
// @Param X-Scope-ID header string false "创作空间"
// @Success 204
// @Router /api/ai/grants/{id} [delete]
func (h *Handler) revokeGrant(c *gin.Context) {
	sid, err := scope(c)
	id, e := uuid.Parse(c.Param("id"))
	if err != nil || e != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	if err = h.store(identityhttp.Principal(c)).RevokeGrant(c.Request.Context(), sid, id); err != nil {
		problem(c, err)
		return
	}
	c.Status(204)
}

// reconcile 按原供应商证据处理未知调用.
// @Summary 按原供应商证据处理未知调用
// @ID reconcileAiRequest
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 200 {object} object
// @Router /api/ai/admin/reconciliations [post]
func (h *Handler) reconcile(c *gin.Context) {
	var in pg.Reconciliation
	if c.ShouldBindJSON(&in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	out, err := h.store(identityhttp.Principal(c)).Reconcile(c.Request.Context(), in)
	if err != nil {
		problem(c, err)
		return
	}
	out.Receipt = nil
	c.JSON(200, out)
}

// configuration 读取模型、路由与脱敏账号池.
// @Summary 读取模型、路由与脱敏账号池
// @ID readAiConfiguration
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Success 200 {object} object
// @Router /api/ai/admin/configuration [get]
func (h *Handler) configuration(c *gin.Context) {
	out, err := h.store(identityhttp.Principal(c)).Configuration(c.Request.Context())
	if err != nil {
		problem(c, err)
		return
	}
	out["media_drivers"] = provider.MediaDriverOptions()
	c.JSON(200, out)
}

// budget 设置项目积分上限.
// @Summary 设置项目积分上限
// @ID setAiProjectBudget
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 204
// @Router /api/ai/admin/budgets [put]
func (h *Handler) budget(c *gin.Context) {
	var in struct {
		Scope   uuid.UUID `json:"scope_id"`
		Project uuid.UUID `json:"project_id"`
		Credits int64     `json:"credits"`
	}
	if c.ShouldBindJSON(&in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	if err := h.store(identityhttp.Principal(c)).SetBudget(c.Request.Context(), in.Scope, in.Project, in.Credits); err != nil {
		problem(c, err)
		return
	}
	c.Status(204)
}

// chat OpenAI Chat 原生调用.
// @Summary OpenAI Chat 原生调用
// @Param Idempotency-Key header string true "同一逻辑调用的稳定唯一键"
// @ID createAiChatCompletion
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 200 {object} object
// @Router /api/ai/v1/chat/completions [post]
func (h *Handler) chat(c *gin.Context) { h.native("chat")(c) }

// responses OpenAI Responses 原生调用.
// @Summary OpenAI Responses 原生调用
// @Param Idempotency-Key header string true "同一逻辑调用的稳定唯一键"
// @ID createAiResponse
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 200 {object} object
// @Router /api/ai/v1/responses [post]
func (h *Handler) responses(c *gin.Context) { h.native("responses")(c) }

// messages Anthropic Messages 原生调用.
// @Summary Anthropic Messages 原生调用
// @Param Idempotency-Key header string true "同一逻辑调用的稳定唯一键"
// @ID createAiMessage
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "合同或协议原始 JSON；仅支持明确公布的字段"
// @Success 200 {object} object
// @Router /api/ai/v1/messages [post]
func (h *Handler) messages(c *gin.Context) { h.native("messages")(c) }
