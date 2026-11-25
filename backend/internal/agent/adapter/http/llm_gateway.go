package http

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  json.RawMessage `json:"messages"`
	Tools     json.RawMessage `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

// registerLLMGateway exposes the Go unified channel pass-through endpoints for Mastra / Vercel AI SDK.
func (h *BusinessHandler) registerLLMGateway(router *gin.Engine) {
	g := router.Group("/internal/agent-gateway", func(c *gin.Context) {
		if len(h.token) < 32 ||
			subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+h.token)) != 1 ||
			c.GetHeader("X-Agent-Audience") != "agent-business" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "service_identity_required"})
			return
		}
		c.Header("Cache-Control", "no-store")
		select {
		case h.concurrent <- struct{}{}:
			defer func() { <-h.concurrent }()
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "agent_busy"})
		}
	})
	g.POST("/v1/chat/completions", h.handleGatewayChatCompletions)
}

func (h *BusinessHandler) handleGatewayChatCompletions(c *gin.Context) {
	if h.capabilities.Model == nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	var req openAIChatRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" || len(req.Messages) == 0 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	actorID, errA := uuid.Parse(c.GetHeader("X-Glowloom-Actor-Id"))
	orgID, errO := uuid.Parse(c.GetHeader("X-Glowloom-Org-Id"))
	scopeID, errS := uuid.Parse(c.GetHeader("X-Glowloom-Scope-Id"))
	projectID, errP := uuid.Parse(c.GetHeader("X-Glowloom-Project-Id"))
	if errA != nil || errO != nil || errS != nil || errP != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	rawInput, err := json.Marshal(map[string]any{
		"model_key":  req.Model,
		"messages":   req.Messages,
		"tools":      req.Tools,
		"max_tokens": maxTokens,
	})
	if err != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	scope := domain.Scope{ActorID: actorID, OrgID: orgID, ScopeID: scopeID, ProjectID: projectID}
	callID := uuid.New()
	res, err := h.capabilities.Model(c.Request.Context(), scope, callID, rawInput, 0)
	if err != nil {
		send(c, nil, err)
		return
	}
	if res.Status != domain.CallStatusCompleted {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": res.Status,
				"type":    "gateway_error",
			},
		})
		return
	}
	var parsed struct {
		Output     string `json:"output"`
		ModelKey   string `json:"model_key"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens      int64  `json:"input_tokens"`
			OutputTokens     int64  `json:"output_tokens"`
			CacheReadTokens  *int64 `json:"cache_read_tokens,omitempty"`
			CacheWriteTokens *int64 `json:"cache_write_tokens,omitempty"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(res.Result, &parsed)
	finishReason := parsed.StopReason
	if finishReason == "" {
		finishReason = "stop"
	}
	c.JSON(http.StatusOK, gin.H{
		"id":      "chatcmpl-" + callID.String(),
		"object":  "chat.completion",
		"model":   req.Model,
		"choices": []gin.H{{"index": 0, "message": gin.H{"role": "assistant", "content": parsed.Output}, "finish_reason": finishReason}},
		"usage": gin.H{
			"prompt_tokens":     parsed.Usage.InputTokens,
			"completion_tokens": parsed.Usage.OutputTokens,
			"total_tokens":      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	})
}
