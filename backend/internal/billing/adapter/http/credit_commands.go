package http

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// CreditCommandService accepts and reads authorized credit commands.
type CreditCommandService interface {
	Submit(context.Context, application.CreditCommand, string) (application.CreditCommandReceipt, error)
	Read(context.Context, string, uuid.UUID, uuid.UUID) (application.CreditCommandReceipt, error)
}

// CreditCommandHandler exposes the platform-admin credit command API.
type CreditCommandHandler struct{ service CreditCommandService }

// NewCreditCommandHandler constructs the credit command HTTP adapter.
func NewCreditCommandHandler(service CreditCommandService) *CreditCommandHandler {
	return &CreditCommandHandler{service: service}
}

// Register mounts command submission and receipt routes on a protected group.
func (h *CreditCommandHandler) Register(group *gin.RouterGroup) {
	group.POST("/billing/scopes/:scopeID/credit-commands", h.Submit)
	group.GET("/billing/scopes/:scopeID/credit-commands/:commandID", h.Read)
}

type creditCommandRequest struct {
	Action        application.CreditAction `json:"action" binding:"required"`
	SourceID      string                   `json:"source_id"`
	ReferenceID   string                   `json:"reference_id"`
	AmountCredits int64                    `json:"amount_credits" binding:"required,gt=0"`
	ExpiresAt     *time.Time               `json:"expires_at,omitempty"`
	ModelKey      *string                  `json:"model_key,omitempty"`
	Reason        string                   `json:"reason" binding:"required"`
}

// Submit validates and durably accepts one idempotent credit command.
func (h *CreditCommandHandler) Submit(c *gin.Context) {
	actor := identityhttp.Principal(c)
	if actor.PlatformRole != "platform_admin" {
		httpapi.WriteProblem(c, http.StatusNotFound, "not_found", nil)
		return
	}
	scopeID, ok := parseCreditUUID(c, "scopeID", "invalid_scope")
	if !ok {
		return
	}
	if h.service == nil {
		httpapi.WriteProblem(c, http.StatusServiceUnavailable, "dependency_unavailable", nil)
		return
	}
	var body creditCommandRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "invalid_credit_command", nil)
		return
	}
	if body.AmountCredits > math.MaxInt64/domain.MicrosPerCredit {
		httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "invalid_credit_amount", nil)
		return
	}
	var sourceID uuid.UUID
	var referenceID *uuid.UUID
	if body.ReferenceID != "" {
		id, err := uuid.Parse(body.ReferenceID)
		if err != nil || id == uuid.Nil || id.String() != body.ReferenceID {
			httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "invalid_reference_id", nil)
			return
		}
		referenceID = &id
	}
	if body.SourceID != "" {
		id, err := uuid.Parse(body.SourceID)
		if err != nil || id == uuid.Nil || id.String() != body.SourceID {
			httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "invalid_source_id", nil)
			return
		}
		sourceID = id
	} else {
		switch body.Action {
		case application.CreditCommit, application.CreditRelease:
			if referenceID != nil {
				sourceID = *referenceID
			}
		case application.CreditGrant, application.CreditDebit, application.CreditFreeze:
			httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "missing_source_id", nil)
			return
		}
	}
	command := application.CreditCommand{
		CallerID: actor.ID.String(), IdempotencyKey: c.GetHeader("Idempotency-Key"), ScopeID: scopeID,
		Action: body.Action, SourceID: sourceID, ReferenceID: referenceID,
		AmountMicros: body.AmountCredits * domain.MicrosPerCredit, ExpiresAt: body.ExpiresAt, ModelKey: body.ModelKey, Reason: body.Reason,
	}
	receipt, err := h.service.Submit(c.Request.Context(), command, c.ClientIP())
	if err != nil {
		writeCreditCommandError(c, err)
		return
	}
	c.Header("Location", "/api/platform/billing/scopes/"+scopeID.String()+"/credit-commands/"+receipt.ID.String())
	c.JSON(http.StatusAccepted, receipt)
}

// Read returns the caller's receipt for one accepted command.
func (h *CreditCommandHandler) Read(c *gin.Context) {
	actor := identityhttp.Principal(c)
	if actor.PlatformRole != "platform_admin" {
		httpapi.WriteProblem(c, http.StatusNotFound, "not_found", nil)
		return
	}
	scopeID, ok := parseCreditUUID(c, "scopeID", "invalid_scope")
	if !ok {
		return
	}
	commandID, ok := parseCreditUUID(c, "commandID", "invalid_command_id")
	if !ok {
		return
	}
	if h.service == nil {
		httpapi.WriteProblem(c, http.StatusServiceUnavailable, "dependency_unavailable", nil)
		return
	}
	receipt, err := h.service.Read(c.Request.Context(), actor.ID.String(), scopeID, commandID)
	if err != nil {
		writeCreditCommandError(c, err)
		return
	}
	c.JSON(http.StatusOK, receipt)
}

func parseCreditUUID(c *gin.Context, name, code string) (uuid.UUID, bool) {
	raw := c.Param(name)
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || id.String() != raw {
		httpapi.WriteProblem(c, http.StatusUnprocessableEntity, code, nil)
		return uuid.Nil, false
	}
	return id, true
}

func writeCreditCommandError(c *gin.Context, err error) {
	var rateLimited *application.CreditRateLimitedError
	switch {
	case errors.Is(err, application.ErrInvalidCreditCommand):
		httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "invalid_credit_command", nil)
	case errors.Is(err, application.ErrCreditIdempotencyConflict):
		httpapi.WriteProblem(c, http.StatusConflict, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrCreditCommandConflict):
		httpapi.WriteProblem(c, http.StatusConflict, "credit_command_conflict", nil)
	case errors.Is(err, application.ErrCreditCommandNotFound):
		httpapi.WriteProblem(c, http.StatusNotFound, "not_found", nil)
	case errors.As(err, &rateLimited):
		seconds := int(math.Ceil(rateLimited.RetryAfter.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(seconds))
		httpapi.WriteProblem(c, http.StatusTooManyRequests, "credit_rate_limited", nil)
	default:
		httpapi.LogDependencyError(c, err)
		httpapi.WriteProblem(c, http.StatusServiceUnavailable, "dependency_unavailable", nil)
	}
}
