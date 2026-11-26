package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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

// resolveMCPCallID derives a deterministic UUID when the caller supplies
// operationId/operation_id so crash-replayed billable tool calls hit Go's
// idempotent execution ledger instead of double-charging.
func resolveMCPCallID(args json.RawMessage) uuid.UUID {
	var envelope struct {
		OperationID      string `json:"operationId"`
		OperationIDSnake string `json:"operation_id"`
	}
	if json.Unmarshal(args, &envelope) == nil {
		rawOp := strings.TrimSpace(envelope.OperationID)
		if rawOp == "" {
			rawOp = strings.TrimSpace(envelope.OperationIDSnake)
		}
		if rawOp != "" {
			if parsed, err := uuid.Parse(rawOp); err == nil {
				return parsed
			}
			return uuid.NewSHA1(uuid.NameSpaceOID, []byte("glowloom:mcp:op:"+rawOp))
		}
	}
	return uuid.New()
}

func isTerminalMCPTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "ready", "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

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
			callID := resolveMCPCallID(args)
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

	h.registerBuiltInHarnessMCPTools(server)
	return server
}

func (h *BusinessHandler) registerBuiltInHarnessMCPTools(server *mcp.Server) {
	// 1. Built-in bounded long-poll task_wait tool (<= 25s slice) backed by media_get_task/task_get.
	var taskGetTool *app.Tool
	for i := range h.capabilities.Tools {
		name := h.capabilities.Tools[i].Descriptor.Name
		if name == "media_get_task" || name == "task_get" {
			t := h.capabilities.Tools[i]
			taskGetTool = &t
			break
		}
	}
	if taskGetTool != nil && taskGetTool.Execute != nil {
		waitTool := &mcp.Tool{
			Name:        "task_wait",
			Description: "Deterministically wait up to 25s for async media task IDs to finish without burning LLM turns",
			InputSchema: json.RawMessage(`{"type":"object","required":["taskIds"],"properties":{"taskIds":{"type":"array","items":{"type":"string"}},"timeoutMs":{"type":"integer"}}}`),
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:   true,
				IdempotentHint: true,
			},
			Meta: mcp.Meta{"effect": "read", "billable": false},
		}
		server.AddTool(waitTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			caller, _ := ctx.Value(mcpIdentityKey{}).(mcpCallerIdentity)
			if !caller.valid {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "missing or invalid glowloom identity headers"}},
				}, nil
			}
			var in struct {
				TaskIDs   []string `json:"taskIds"`
				TimeoutMs int      `json:"timeoutMs"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &in); err != nil || len(in.TaskIDs) == 0 {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid task_wait arguments: taskIds required"}},
				}, nil
			}
			sliceMs := in.TimeoutMs
			if sliceMs <= 0 {
				sliceMs = 5000
			}
			if sliceMs > 25000 {
				sliceMs = 25000
			}
			deadline := time.Now().Add(time.Duration(sliceMs) * time.Millisecond)
			pollInterval := 250 * time.Millisecond
			if sliceMs < 500 {
				pollInterval = 25 * time.Millisecond
			}

			var latest []map[string]any
			for {
				latest = make([]map[string]any, 0, len(in.TaskIDs))
				allDone := true
				for _, tid := range in.TaskIDs {
					argBytes, _ := json.Marshal(map[string]string{"task_id": tid, "taskId": tid})
					res, err := taskGetTool.Execute(ctx, caller.scope, uuid.New(), argBytes)
					entry := map[string]any{"id": tid, "status": "running"}
					if err != nil {
						entry["status"] = "failed"
						entry["error"] = err.Error()
					} else if len(res.Result) > 0 {
						var decoded map[string]any
						if json.Unmarshal(res.Result, &decoded) == nil {
							for k, v := range decoded {
								entry[k] = v
							}
							if _, hasID := entry["id"]; !hasID {
								entry["id"] = tid
							}
						}
					}
					statusStr, _ := entry["status"].(string)
					if !isTerminalMCPTaskStatus(statusStr) {
						allDone = false
					}
					latest = append(latest, entry)
				}
				if allDone || time.Now().After(deadline) {
					break
				}
				timer := time.NewTimer(pollInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{&mcp.TextContent{Text: ctx.Err().Error()}},
					}, nil
				case <-timer.C:
				}
			}

			payload, _ := json.Marshal(map[string]any{"tasks": latest})
			return &mcp.CallToolResult{
				IsError: false,
				Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}},
			}, nil
		})
	}

	// 2. Built-in skill_load_bundle tool for zero-fetch private skill loading over MCP.
	if h.skills != nil {
		skillTool := &mcp.Tool{
			Name:        "skill_load_bundle",
			Description: "Load a pinned private immutable skill bundle within the authorized run grant",
			InputSchema: json.RawMessage(`{"type":"object","required":["skill_id","version","resource_hash"],"properties":{"root_run_id":{"type":"string"},"submission_id":{"type":"string"},"skill_id":{"type":"string"},"version":{"type":"string"},"resource_hash":{"type":"string"}}}`),
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:   true,
				IdempotentHint: true,
			},
			Meta: mcp.Meta{"effect": "read", "billable": false},
		}
		server.AddTool(skillTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			caller, _ := ctx.Value(mcpIdentityKey{}).(mcpCallerIdentity)
			if !caller.valid {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "missing or invalid glowloom identity headers"}},
				}, nil
			}
			var in struct {
				RootRunID    string `json:"root_run_id"`
				SkillID      string `json:"skill_id"`
				Version      string `json:"version"`
				ResourceHash string `json:"resource_hash"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &in); err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid skill_load_bundle arguments"}},
				}, nil
			}
			skillID, err := uuid.Parse(in.SkillID)
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid skill_id"}},
				}, nil
			}
			runID := caller.runID
			if parsedRun, err := uuid.Parse(in.RootRunID); err == nil {
				runID = parsedRun
			}
			_, grant, err := h.executionGrant(ctx, runID, caller.scope)
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				}, nil
			}
			content, err := h.skills.ReadSkill(ctx, grant, skillID, in.Version, in.ResourceHash, "")
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				}, nil
			}
			payload, _ := json.Marshal(content)
			return &mcp.CallToolResult{
				IsError: false,
				Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}},
			}, nil
		})
	}
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
