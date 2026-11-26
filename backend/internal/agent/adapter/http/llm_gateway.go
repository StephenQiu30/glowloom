package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

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

type openAIResponsesRequest struct {
	Model  string          `json:"model"`
	Input  json.RawMessage `json:"input"`
	Tools  json.RawMessage `json:"tools,omitempty"`
	Stream bool            `json:"stream,omitempty"`
}

type anthropicMessagesRequest struct {
	Model     string          `json:"model"`
	System    json.RawMessage `json:"system,omitempty"`
	Messages  json.RawMessage `json:"messages"`
	Tools     json.RawMessage `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Thinking  json.RawMessage `json:"thinking,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

type googleGenerateContentRequest struct {
	Contents          json.RawMessage `json:"contents"`
	SystemInstruction json.RawMessage `json:"systemInstruction,omitempty"`
	Tools             json.RawMessage `json:"tools,omitempty"`
	GenerationConfig  json.RawMessage `json:"generationConfig,omitempty"`
}

type gatewayModelResult struct {
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
	g.POST("/openai/v1/chat/completions", h.handleGatewayChatCompletions)
	g.POST("/openai/v1/responses", h.handleGatewayOpenAIResponses)
	g.POST("/anthropic/v1/messages", h.handleGatewayAnthropicMessages)
	g.POST("/google/v1beta/models/*action", h.handleGatewayGoogleGenerateContent)
}

func (h *BusinessHandler) executeGatewayCall(
	c *gin.Context,
	protocol string,
	modelKey string,
	messages json.RawMessage,
	tools json.RawMessage,
	extra map[string]any,
	stream bool,
) (uuid.UUID, gatewayModelResult, bool) {
	if h.capabilities.Model == nil {
		send(c, nil, domain.ErrUnavailable)
		return uuid.Nil, gatewayModelResult{}, false
	}
	actorID, errA := uuid.Parse(c.GetHeader("X-Glowloom-Actor-Id"))
	orgID, errO := uuid.Parse(c.GetHeader("X-Glowloom-Org-Id"))
	scopeID, errS := uuid.Parse(c.GetHeader("X-Glowloom-Scope-Id"))
	projectID, errP := uuid.Parse(c.GetHeader("X-Glowloom-Project-Id"))
	if errA != nil || errO != nil || errS != nil || errP != nil {
		send(c, nil, domain.ErrInvalid)
		return uuid.Nil, gatewayModelResult{}, false
	}
	scope := domain.Scope{ActorID: actorID, OrgID: orgID, ScopeID: scopeID, ProjectID: projectID}
	callID := uuid.New()
	if supplied, parseErr := uuid.Parse(c.GetHeader("X-Glowloom-Call-Id")); parseErr == nil {
		callID = supplied
	}
	runID, _ := uuid.Parse(c.GetHeader("X-Glowloom-Run-Id"))

	var result domain.CallResult
	var err error
	if h.store == nil {
		payload := map[string]any{
			"protocol":  protocol,
			"model_key": modelKey,
			"messages":  messages,
		}
		if len(tools) > 0 {
			payload["tools"] = tools
		}
		for k, v := range extra {
			payload[k] = v
		}
		input, _ := json.Marshal(payload)
		result, err = h.capabilities.Model(c.Request.Context(), scope, callID, input, 0)
	} else {
		var authority domain.Authority
		var grant domain.Grant
		authority, grant, err = h.executionGrant(c.Request.Context(), runID, scope)
		if err != nil {
			send(c, nil, err)
			return uuid.Nil, gatewayModelResult{}, false
		}
		var selection struct {
			ModelKey  string          `json:"model_key"`
			ReleaseID uuid.UUID       `json:"release_id"`
			MaxTokens int             `json:"max_tokens"`
			Params    json.RawMessage `json:"params"`
		}
		if json.Unmarshal(authority.ModelSelection, &selection) != nil || selection.ModelKey != modelKey || selection.MaxTokens <= 0 || stream {
			send(c, nil, domain.ErrInvalid)
			return uuid.Nil, gatewayModelResult{}, false
		}
		payload := map[string]any{
			"protocol":   protocol,
			"model_key":  selection.ModelKey,
			"release_id": selection.ReleaseID,
			"params":     selection.Params,
			"messages":   messages,
			"max_tokens": selection.MaxTokens,
		}
		if len(tools) > 0 {
			if protocol == "openai-chat" {
				var openAITools []struct {
					Type     string          `json:"type"`
					Function json.RawMessage `json:"function"`
				}
				if json.Unmarshal(tools, &openAITools) != nil {
					send(c, nil, domain.ErrInvalid)
					return uuid.Nil, gatewayModelResult{}, false
				}
				functions := make([]json.RawMessage, 0, len(openAITools))
				for _, tool := range openAITools {
					if tool.Type != "function" || len(tool.Function) == 0 {
						send(c, nil, domain.ErrInvalid)
						return uuid.Nil, gatewayModelResult{}, false
					}
					functions = append(functions, tool.Function)
				}
				payload["tools"] = functions
			} else {
				payload["tools"] = tools
			}
		}
		for k, v := range extra {
			payload[k] = v
		}
		raw, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			send(c, nil, domain.ErrInvalid)
			return uuid.Nil, gatewayModelResult{}, false
		}
		in := domain.CallInput{
			Grant:     grant,
			Input:     raw,
			MaxMicros: grant.AvailableMicros,
			Call: domain.CallIdentity{
				TaskID:          runID.String(),
				CallID:          callID.String(),
				CheckpointID:    grant.SubmissionID.String(),
				Attempt:         1,
				ContractVersion: "v1",
				InputHash:       domain.Hash(raw),
			},
		}
		var receipt domain.CallReceipt
		receipt, err = h.store.ExecuteCall(c.Request.Context(), "model", "chat", in,
			func(ctx context.Context, scope domain.Scope, id uuid.UUID) (domain.CallResult, error) {
				return h.capabilities.Model(ctx, scope, id, raw, in.MaxMicros)
			})
		result = receipt.CallResult
	}
	if err != nil {
		send(c, nil, err)
		return uuid.Nil, gatewayModelResult{}, false
	}
	if result.Status != domain.CallStatusCompleted {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": result.Status,
				"type":    "gateway_error",
			},
		})
		return uuid.Nil, gatewayModelResult{}, false
	}
	var parsed gatewayModelResult
	_ = json.Unmarshal(result.Result, &parsed)
	return callID, parsed, true
}

func (h *BusinessHandler) handleGatewayChatCompletions(c *gin.Context) {
	var req openAIChatRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" || len(req.Messages) == 0 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	callID, parsed, ok := h.executeGatewayCall(c, "openai-chat", req.Model, req.Messages, req.Tools, nil, req.Stream)
	if !ok {
		return
	}
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
	usage := gin.H{
		"prompt_tokens":     parsed.Usage.InputTokens,
		"completion_tokens": parsed.Usage.OutputTokens,
		"total_tokens":      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
	}
	if parsed.Usage.CacheReadTokens != nil {
		usage["prompt_tokens_details"] = gin.H{"cached_tokens": *parsed.Usage.CacheReadTokens}
	}
	c.JSON(http.StatusOK, gin.H{
		"id":      "chatcmpl-" + callID.String(),
		"object":  "chat.completion",
		"model":   req.Model,
		"choices": []gin.H{{"index": 0, "message": message, "finish_reason": finishReason}},
		"usage":   usage,
	})
}

func (h *BusinessHandler) handleGatewayOpenAIResponses(c *gin.Context) {
	var req openAIResponsesRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" || len(req.Input) == 0 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	callID, parsed, ok := h.executeGatewayCall(c, "openai-responses", req.Model, req.Input, req.Tools, nil, req.Stream)
	if !ok {
		return
	}
	outputItems := make([]gin.H, 0, 1+len(parsed.ToolCalls))
	if parsed.Output != "" {
		outputItems = append(outputItems, gin.H{
			"id":     "msg_" + callID.String(),
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []gin.H{
				{"type": "output_text", "text": parsed.Output, "annotations": []any{}},
			},
		})
	}
	for _, call := range parsed.ToolCalls {
		outputItems = append(outputItems, gin.H{
			"id":        "fc_" + call.ID,
			"type":      "function_call",
			"call_id":   call.ID,
			"name":      call.Name,
			"arguments": string(call.Arguments),
			"status":    "completed",
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"id":     "resp_" + callID.String(),
		"object": "response",
		"status": "completed",
		"model":  req.Model,
		"output": outputItems,
		"usage": gin.H{
			"input_tokens":  parsed.Usage.InputTokens,
			"output_tokens": parsed.Usage.OutputTokens,
			"total_tokens":  parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	})
}

func (h *BusinessHandler) handleGatewayAnthropicMessages(c *gin.Context) {
	var req anthropicMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" || len(req.Messages) == 0 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	extra := map[string]any{}
	if len(req.System) > 0 {
		extra["system"] = req.System
	}
	if len(req.Metadata) > 0 {
		extra["metadata"] = req.Metadata
	}
	if len(req.Thinking) > 0 {
		extra["thinking"] = req.Thinking
	}
	callID, parsed, ok := h.executeGatewayCall(c, "anthropic-messages", req.Model, req.Messages, req.Tools, extra, req.Stream)
	if !ok {
		return
	}
	content := make([]gin.H, 0, 1+len(parsed.ToolCalls))
	if parsed.Output != "" {
		content = append(content, gin.H{"type": "text", "text": parsed.Output})
	}
	stopReason := "end_turn"
	if len(parsed.ToolCalls) > 0 {
		stopReason = "tool_use"
		for _, call := range parsed.ToolCalls {
			var inputObj any = map[string]any{}
			if len(call.Arguments) > 0 {
				_ = json.Unmarshal(call.Arguments, &inputObj)
			}
			content = append(content, gin.H{
				"type":  "tool_use",
				"id":    call.ID,
				"name":  call.Name,
				"input": inputObj,
			})
		}
	}
	usage := gin.H{
		"input_tokens":  parsed.Usage.InputTokens,
		"output_tokens": parsed.Usage.OutputTokens,
	}
	if parsed.Usage.CacheReadTokens != nil {
		usage["cache_read_input_tokens"] = *parsed.Usage.CacheReadTokens
	}
	if parsed.Usage.CacheWriteTokens != nil {
		usage["cache_creation_input_tokens"] = *parsed.Usage.CacheWriteTokens
	}
	c.JSON(http.StatusOK, gin.H{
		"id":          "msg_" + callID.String(),
		"type":        "message",
		"role":        "assistant",
		"model":       req.Model,
		"content":     content,
		"stop_reason": stopReason,
		"usage":       usage,
	})
}

func (h *BusinessHandler) handleGatewayGoogleGenerateContent(c *gin.Context) {
	action := strings.TrimPrefix(c.Param("action"), "/")
	modelName, method, found := strings.Cut(action, ":")
	if !found || modelName == "" || (method != "generateContent" && method != "streamGenerateContent") {
		send(c, nil, domain.ErrInvalid)
		return
	}
	var req googleGenerateContentRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Contents) == 0 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	extra := map[string]any{}
	if len(req.SystemInstruction) > 0 {
		extra["system_instruction"] = req.SystemInstruction
	}
	if len(req.GenerationConfig) > 0 {
		extra["generation_config"] = req.GenerationConfig
	}
	_, parsed, ok := h.executeGatewayCall(c, "google-generative-ai", modelName, req.Contents, req.Tools, extra, method == "streamGenerateContent")
	if !ok {
		return
	}
	parts := make([]gin.H, 0, 1+len(parsed.ToolCalls))
	if parsed.Output != "" {
		parts = append(parts, gin.H{"text": parsed.Output})
	}
	for _, call := range parsed.ToolCalls {
		var argsObj any = map[string]any{}
		if len(call.Arguments) > 0 {
			_ = json.Unmarshal(call.Arguments, &argsObj)
		}
		parts = append(parts, gin.H{
			"functionCall": gin.H{
				"name": call.Name,
				"args": argsObj,
			},
		})
	}
	usageMetadata := gin.H{
		"promptTokenCount":     parsed.Usage.InputTokens,
		"candidatesTokenCount": parsed.Usage.OutputTokens,
		"totalTokenCount":      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
	}
	if parsed.Usage.CacheReadTokens != nil {
		usageMetadata["cachedContentTokenCount"] = *parsed.Usage.CacheReadTokens
	}
	c.JSON(http.StatusOK, gin.H{
		"candidates": []gin.H{
			{
				"content": gin.H{
					"role":  "model",
					"parts": parts,
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": usageMetadata,
	})
}
