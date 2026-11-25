package http

import (
	"context"
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
	scope := domain.Scope{ActorID: actorID, OrgID: orgID, ScopeID: scopeID, ProjectID: projectID}
	callID := uuid.New()
	runID, _ := uuid.Parse(c.GetHeader("X-Glowloom-Run-Id"))
	var result domain.CallResult
	var err error
	if h.store == nil {
		// Isolated capability handlers have no admission store.
		input, _ := json.Marshal(map[string]any{"model_key": req.Model, "messages": req.Messages})
		result, err = h.capabilities.Model(c.Request.Context(), scope, callID, input, 0)
	} else {
		var authority domain.Authority
		var grant domain.Grant
		authority, grant, err = h.executionGrant(c.Request.Context(), runID, scope)
		if err != nil {
			send(c, nil, err)
			return
		}
		var selection struct {
			ModelKey  string          `json:"model_key"`
			ReleaseID uuid.UUID       `json:"release_id"`
			MaxTokens int             `json:"max_tokens"`
			Params    json.RawMessage `json:"params"`
		}
		if json.Unmarshal(authority.ModelSelection, &selection) != nil || selection.ModelKey != req.Model || selection.MaxTokens <= 0 || req.Stream {
			send(c, nil, domain.ErrInvalid)
			return
		}
		var tools []struct {
			Type     string          `json:"type"`
			Function json.RawMessage `json:"function"`
		}
		if len(req.Tools) > 0 && json.Unmarshal(req.Tools, &tools) != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		functions := make([]json.RawMessage, 0, len(tools))
		for _, tool := range tools {
			if tool.Type != "function" || len(tool.Function) == 0 {
				send(c, nil, domain.ErrInvalid)
				return
			}
			functions = append(functions, tool.Function)
		}
		raw, encodeErr := json.Marshal(map[string]any{
			"model_key": selection.ModelKey, "release_id": selection.ReleaseID, "params": selection.Params,
			"messages": req.Messages, "tools": functions, "max_tokens": selection.MaxTokens,
		})
		if encodeErr != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		if supplied, parseErr := uuid.Parse(c.GetHeader("X-Glowloom-Call-Id")); parseErr == nil {
			callID = supplied
		}
		in := domain.CallInput{Grant: grant, Input: raw, MaxMicros: grant.AvailableMicros,
			Call: domain.CallIdentity{TaskID: runID.String(), CallID: callID.String(), CheckpointID: grant.SubmissionID.String(), Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}}
		var receipt domain.CallReceipt
		receipt, err = h.store.ExecuteCall(c.Request.Context(), "model", "chat", in,
			func(ctx context.Context, scope domain.Scope, id uuid.UUID) (domain.CallResult, error) {
				return h.capabilities.Model(ctx, scope, id, raw, in.MaxMicros)
			})
		result = receipt.CallResult
	}
	if err != nil {
		send(c, nil, err)
		return
	}
	if result.Status != domain.CallStatusCompleted {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": result.Status,
				"type":    "gateway_error",
			},
		})
		return
	}
	var parsed struct {
		Output     string `json:"output"`
		ModelKey   string `json:"model_key"`
		StopReason string `json:"stop_reason"`
		ToolCalls  []struct {
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"tool_calls"`
		Usage struct {
			InputTokens      int64  `json:"input_tokens"`
			OutputTokens     int64  `json:"output_tokens"`
			CacheReadTokens  *int64 `json:"cache_read_tokens,omitempty"`
			CacheWriteTokens *int64 `json:"cache_write_tokens,omitempty"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(result.Result, &parsed)
	finishReason := parsed.StopReason
	if finishReason == "" {
		finishReason = "stop"
	}
	message := gin.H{"role": "assistant", "content": parsed.Output}
	if len(parsed.ToolCalls) > 0 {
		calls := make([]gin.H, 0, len(parsed.ToolCalls))
		for _, call := range parsed.ToolCalls {
			calls = append(calls, gin.H{"type": "function", "id": call.ID, "function": gin.H{"name": call.Name, "arguments": string(call.Arguments)}})
		}
		message["tool_calls"] = calls
		finishReason = "tool_calls"
	}
	c.JSON(http.StatusOK, gin.H{
		"id":      "chatcmpl-" + callID.String(),
		"object":  "chat.completion",
		"model":   req.Model,
		"choices": []gin.H{{"index": 0, "message": message, "finish_reason": finishReason}},
		"usage": gin.H{
			"prompt_tokens":     parsed.Usage.InputTokens,
			"completion_tokens": parsed.Usage.OutputTokens,
			"total_tokens":      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	})
}
