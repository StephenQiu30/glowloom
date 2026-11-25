package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

type mcpIdentityKey struct{}

type mcpCallerIdentity struct {
	scope domain.Scope
	runID uuid.UUID
	valid bool
}

func boolPtr(v bool) *bool { return &v }

func annotationsForTool(desc app.ToolDescriptor) *mcp.ToolAnnotations {
	readOnly := desc.Effect == "read"
	destructive := desc.Effect == "billable"
	openWorld := desc.Effect == "billable"
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: boolPtr(destructive),
		IdempotentHint:  desc.Replay == "idempotent",
		OpenWorldHint:   boolPtr(openWorld),
	}
}

func (h *BusinessHandler) newOfficialMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "glowloom-business-mcp",
		Version: "1.0.0",
	}, nil)

	for _, t := range h.capabilities.Tools {
		tool := t
		schema := tool.Descriptor.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		mcpTool := &mcp.Tool{
			Name:        tool.Descriptor.Name,
			Description: tool.Descriptor.Description,
			InputSchema: schema,
			Annotations: annotationsForTool(tool.Descriptor),
			Meta: mcp.Meta{
				"effect":                 tool.Descriptor.Effect,
				"roles":                  tool.Descriptor.Roles,
				"billable":               tool.Descriptor.Effect == "billable",
				"requiresOptimisticLock": tool.Descriptor.Effect == "write",
			},
		}
		server.AddTool(mcpTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			caller, _ := ctx.Value(mcpIdentityKey{}).(mcpCallerIdentity)
			if !caller.valid {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: "missing or invalid glowloom identity headers"},
					},
				}, nil
			}
			args := req.Params.Arguments
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			callID := uuid.New()
			var result domain.CallResult
			var execErr error
			switch {
			case h.atomicTool != nil:
				_, grant, err := h.executionGrant(ctx, caller.runID, caller.scope)
				if err != nil {
					return nil, err
				}
				in := domain.CallInput{Grant: grant, Input: args,
					Call: domain.CallIdentity{TaskID: caller.runID.String(), CallID: callID.String(), CheckpointID: grant.SubmissionID.String(), Attempt: 1, ContractVersion: tool.Descriptor.ContractVersion, InputHash: domain.Hash(args)}}
				receipt, err := h.atomicTool(ctx, tool.Descriptor.Name, in)
				result, execErr = receipt.CallResult, err
			case h.store == nil && tool.Execute != nil:
				result, execErr = tool.Execute(ctx, caller.scope, callID, args)
			default:
				return nil, errors.New("tool executor unavailable")
			}
			if execErr != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: execErr.Error()},
					},
				}, nil
			}
			payload, _ := json.Marshal(result)
			return &mcp.CallToolResult{
				IsError: result.Status != domain.CallStatusCompleted,
				Content: []mcp.Content{
					&mcp.TextContent{Text: string(payload)},
				},
			}, nil
		})
	}
	return server
}

// registerMCP mounts the official MCP Streamable HTTP endpoint (POST/GET/DELETE) for @mastra/mcp.
func (h *BusinessHandler) registerMCP(router *gin.Engine) {
	server := h.newOfficialMCPServer()
	streamableHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless:                  true,
			JSONResponse:               true,
			DisableLocalhostProtection: true,
		},
	)

	handle := func(c *gin.Context) {
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

		actorID, errA := uuid.Parse(c.GetHeader("X-Glowloom-Actor-Id"))
		orgID, errO := uuid.Parse(c.GetHeader("X-Glowloom-Org-Id"))
		scopeID, errS := uuid.Parse(c.GetHeader("X-Glowloom-Scope-Id"))
		projectID, errP := uuid.Parse(c.GetHeader("X-Glowloom-Project-Id"))
		runID, _ := uuid.Parse(c.GetHeader("X-Glowloom-Run-Id"))

		caller := mcpCallerIdentity{
			scope: domain.Scope{ActorID: actorID, OrgID: orgID, ScopeID: scopeID, ProjectID: projectID},
			runID: runID,
			valid: errA == nil && errO == nil && errS == nil && errP == nil,
		}
		reqWithCtx := c.Request.WithContext(context.WithValue(c.Request.Context(), mcpIdentityKey{}, caller))
		streamableHandler.ServeHTTP(c.Writer, reqWithCtx)
	}

	router.POST("/internal/agent-mcp", handle)
	router.GET("/internal/agent-mcp", handle)
	router.DELETE("/internal/agent-mcp", handle)
}
