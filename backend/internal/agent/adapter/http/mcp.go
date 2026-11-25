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

const mcpProtocolVersion = "2025-03-26"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// registerMCP mounts the authenticated internal MCP Streamable HTTP endpoint for @mastra/mcp.
func (h *BusinessHandler) registerMCP(router *gin.Engine) {
	router.POST("/internal/agent-mcp", func(c *gin.Context) {
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
		default:
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "agent_busy"})
			return
		}
		h.handleMCP(c)
	})
}

func (h *BusinessHandler) handleMCP(c *gin.Context) {
	var req mcpRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		c.JSON(http.StatusBadRequest, mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32600, Message: "invalid json-rpc request"},
		})
		return
	}

	switch req.Method {
	case "initialize":
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{"listChanged": false},
				},
				"serverInfo": map[string]any{
					"name":    "glowloom-business-mcp",
					"version": "1.0.0",
				},
			},
		})
	case "notifications/initialized":
		c.Status(http.StatusAccepted)
	case "ping":
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		})
	case "tools/list":
		tools := make([]mcpToolSpec, 0, len(h.capabilities.Tools))
		for _, t := range h.capabilities.Tools {
			schema := t.Descriptor.Parameters
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			tools = append(tools, mcpToolSpec{
				Name:        t.Descriptor.Name,
				Description: t.Descriptor.Description,
				InputSchema: schema,
			})
		}
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": tools},
		})
	case "tools/call":
		h.handleMCPToolCall(c, req)
	default:
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: "method not found"},
		})
	}
}

func (h *BusinessHandler) handleMCPToolCall(c *gin.Context, req mcpRequest) {
	var params mcpCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32602, Message: "invalid tools/call params"},
		})
		return
	}

	actorID, errA := uuid.Parse(c.GetHeader("X-Glowloom-Actor-Id"))
	orgID, errO := uuid.Parse(c.GetHeader("X-Glowloom-Org-Id"))
	scopeID, errS := uuid.Parse(c.GetHeader("X-Glowloom-Scope-Id"))
	projectID, errP := uuid.Parse(c.GetHeader("X-Glowloom-Project-Id"))
	if errA != nil || errO != nil || errS != nil || errP != nil {
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32602, Message: "missing or invalid glowloom identity headers"},
		})
		return
	}
	scope := domain.Scope{ActorID: actorID, OrgID: orgID, ScopeID: scopeID, ProjectID: projectID}
	runID, _ := uuid.Parse(c.GetHeader("X-Glowloom-Run-Id"))

	args := params.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}

	for _, tool := range h.capabilities.Tools {
		if tool.Descriptor.Name != params.Name {
			continue
		}
		if tool.Execute == nil {
			c.JSON(http.StatusOK, mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32000, Message: "tool executor unavailable"},
			})
			return
		}
		callID := uuid.New()
		if runID != uuid.Nil {
			callID = runID
		}
		result, execErr := tool.Execute(c.Request.Context(), scope, callID, args)
		if execErr != nil {
			c.JSON(http.StatusOK, mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"isError": true,
					"content": []map[string]any{
						{"type": "text", "text": execErr.Error()},
					},
				},
			})
			return
		}
		payload, _ := json.Marshal(result)
		c.JSON(http.StatusOK, mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"isError": result.Status != domain.CallStatusCompleted,
				"content": []map[string]any{
					{"type": "text", "text": string(payload)},
				},
			},
		})
		return
	}

	c.JSON(http.StatusOK, mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &mcpError{Code: -32602, Message: "unknown tool: " + params.Name},
	})
}

// Unused import guard for context if needed by future extensions.
var _ context.Context
