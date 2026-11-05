// Package http exposes authenticated current Agent boundaries.
package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// BusinessHandler serves the authenticated internal Agent business boundary.
type BusinessHandler struct {
	concurrent   chan struct{}
	store        app.BusinessStore
	capabilities app.Capabilities
	token        string
	skills       SkillStore
	inputSources app.ConversationSourceReader
	atomicTool   func(*gin.Context, string, domain.CallInput) (domain.CallReceipt, error)
}

// NewBusinessHandler injects the business store and installed capabilities.
func NewBusinessHandler(store app.BusinessStore, capabilities app.Capabilities, token string) *BusinessHandler {
	return &BusinessHandler{store: store, capabilities: capabilities, token: token, concurrent: make(chan struct{}, 64)}
}

// WithAtomicTools injects the transaction-bound business tool executor.
func (h *BusinessHandler) WithAtomicTools(execute func(*gin.Context, string, domain.CallInput) (domain.CallReceipt, error)) *BusinessHandler {
	h.atomicTool = execute
	return h
}
func send(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(200, value)
		return
	}
	status, code := 503, "agent_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 422, "invalid_agent_request"
	case errors.Is(err, promptdomain.ErrInvalidDraft), errors.Is(err, promptdomain.ErrUnsupportedSchema):
		status, code = 422, "invalid_conversation_input"
	case errors.Is(err, promptdomain.ErrReferenceUnavailable):
		status, code = 422, "conversation_reference_unavailable"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "agent_revision_conflict"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "agent_not_found"
	case errors.Is(err, identity.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrGrant):
		status, code = 403, "agent_grant_invalid"
	case errors.Is(err, domain.ErrBlocked):
		status, code = 409, "agent_authorization_blocked"
	case errors.Is(err, domain.ErrTerminal):
		status, code = 409, "agent_run_terminal"
	case errors.Is(err, domain.ErrBudget):
		status, code = 409, "agent_budget_exceeded"
	}
	httpapi.WriteProblem(c, status, code, nil)
}

// Register installs this handler's HTTP routes.
func (h *BusinessHandler) Register(router *gin.Engine) {
	g := router.Group("/internal/agent-business", func(c *gin.Context) {
		if len(h.token) < 32 || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+h.token)) != 1 || c.GetHeader("X-Agent-Audience") != "agent-business" {
			c.AbortWithStatusJSON(401, gin.H{"code": "service_identity_required"})
			return
		}
		c.Header("Cache-Control", "no-store")
		select {
		case h.concurrent <- struct{}{}:
			defer func() { <-h.concurrent }()
			c.Next()
		default:
			c.AbortWithStatusJSON(503, gin.H{"code": "agent_busy"})
		}
	})
	h.registerInputSources(g)
	g.POST("/grants", func(c *gin.Context) {
		var in domain.GrantRequest
		if !httpapi.Decode(c, &in) {
			return
		}
		out, err := h.store.RegisterGrant(c.Request.Context(), in)
		send(c, out, err)
	})
	g.POST("/grants/validate", func(c *gin.Context) {
		var in domain.Grant
		if !httpapi.Decode(c, &in) {
			return
		}
		out, err := h.store.ValidateGrant(c.Request.Context(), in)
		send(c, out, err)
	})
	g.GET("/runs/:rid/decisions", func(c *gin.Context) {
		rid, ok := httpapi.PathUUID(c, "rid")
		if !ok {
			return
		}
		out, err := h.store.CurrentAuthority(c.Request.Context(), rid)
		send(c, out, err)
	})
	g.POST("/runs/:rid/quiesced", func(c *gin.Context) {
		rid, ok := httpapi.PathUUID(c, "rid")
		if !ok {
			return
		}
		var in domain.QuiescedReceipt
		if !httpapi.Decode(c, &in) {
			return
		}
		out, err := h.store.Quiesce(c.Request.Context(), rid, in)
		send(c, out, err)
	})
	g.POST("/runs/:rid/receipts", func(c *gin.Context) {
		rid, ok := httpapi.PathUUID(c, "rid")
		if !ok {
			return
		}
		var in domain.AppliedReceipt
		if !httpapi.Decode(c, &in) {
			return
		}
		err := h.store.Acknowledge(c.Request.Context(), rid, in)
		send(c, gin.H{"accepted": err == nil}, err)
	})
	g.POST("/proposals", func(c *gin.Context) {
		var in domain.ProposalInput
		if !httpapi.Decode(c, &in) {
			return
		}
		out, err := h.store.Propose(c.Request.Context(), in)
		send(c, proposalResponse(out), err)
	})
	h.registerSkills(g)
	g.GET("/tools", h.tools)
	g.POST("/tools/:name/execute", h.tool)
	g.POST("/models/stream", h.model)
	g.GET("/runs/:rid/calls/:call_id", h.call)
	g.POST("/snapshot", func(c *gin.Context) {
		var in domain.Grant
		if !httpapi.Decode(c, &in) {
			return
		}
		grant, err := h.store.ValidateGrant(c.Request.Context(), in)
		if err != nil {
			send(c, nil, err)
			return
		}
		if h.capabilities.Snapshot == nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		out, err := h.capabilities.Snapshot(c.Request.Context(), grant.Scope)
		send(c, out, err)
	})
}
func (h *BusinessHandler) tools(c *gin.Context) {
	out := make([]app.ToolDescriptor, 0, len(h.capabilities.Tools))
	for _, tool := range h.capabilities.Tools {
		out = append(out, tool.Descriptor)
	}
	send(c, out, nil)
}
func (h *BusinessHandler) tool(c *gin.Context) {
	var in domain.CallInput
	if !httpapi.Decode(c, &in) {
		return
	}
	for _, tool := range h.capabilities.Tools {
		if tool.Descriptor.Name != c.Param("name") {
			continue
		}
		if in.Call.ContractVersion != tool.Descriptor.ContractVersion {
			send(c, nil, domain.ErrConflict)
			return
		}
		if h.atomicTool != nil {
			out, err := h.atomicTool(c, tool.Descriptor.Name, in)
			send(c, out, err)
			return
		}
		if tool.Descriptor.Effect != "read" || tool.Execute == nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		out, err := h.store.ExecuteCall(c.Request.Context(), "tool", tool.Descriptor.Name, in, func(ctx context.Context, scope domain.Scope, id uuid.UUID) (domain.CallResult, error) {
			return tool.Execute(ctx, scope, id, in.Input)
		})
		send(c, out, err)
		return
	}
	send(c, nil, domain.ErrNotFound)
}
func (h *BusinessHandler) model(c *gin.Context) {
	var in domain.CallInput
	if !decodeModel(c, &in) {
		return
	}
	if h.capabilities.Model == nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	out, err := h.store.ExecuteCall(c.Request.Context(), "model", "chat", in, func(ctx context.Context, scope domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		return h.capabilities.Model(ctx, scope, id, in.Input, in.MaxMicros)
	})
	send(c, out, err)
}
func (h *BusinessHandler) call(c *gin.Context) {
	rid, ok := httpapi.PathUUID(c, "rid")
	if !ok {
		return
	}
	segment, err := uuid.Parse(c.Query("submission_id"))
	if err != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	attempt, err := strconv.ParseInt(c.Query("attempt"), 10, 64)
	if err != nil || attempt < 1 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	out, err := h.store.ReadCall(c.Request.Context(), rid, segment, domain.CallIdentity{CallID: c.Param("call_id"), TaskID: c.Query("task_id"), CheckpointID: c.Query("checkpoint_id"), Attempt: attempt, ContractVersion: c.Query("contract_version")})
	if err == nil && out.Kind == "model" && (out.Status == "in_flight" || out.Status == "unknown") && h.capabilities.ModelRecover != nil {
		if _, recoverErr := h.capabilities.ModelRecover(c.Request.Context(), out.ReceiptID); recoverErr == nil {
			out, err = h.store.ReadCall(c.Request.Context(), rid, segment, out.Call)
		} else if !errors.Is(recoverErr, domain.ErrUnavailable) && !errors.Is(recoverErr, domain.ErrNotFound) {
			err = recoverErr
		}
	}
	if err == nil && out.Kind == "tool" && (out.Status == "in_flight" || out.Status == "unknown") && h.capabilities.ToolRecover != nil {
		if _, recoverErr := h.capabilities.ToolRecover(c.Request.Context(), out.ReceiptID); recoverErr == nil {
			out, err = h.store.ReadCall(c.Request.Context(), rid, segment, out.Call)
		} else if !errors.Is(recoverErr, domain.ErrUnavailable) && !errors.Is(recoverErr, domain.ErrNotFound) {
			err = recoverErr
		}
	}
	send(c, out, err)
}

func proposalResponse(p domain.Proposal) any {
	return gin.H{"proposal_id": p.ProposalID, "root_run_id": p.RootRunID, "candidate_hash": p.CandidateHash, "authorization_revision": p.AuthorizationRevision, "candidate": p.Candidate, "canvas_revision": p.CanvasRevision, "price_version": p.PriceVersion, "budget_micros": p.BudgetMicros, "control": p.Control}
}
