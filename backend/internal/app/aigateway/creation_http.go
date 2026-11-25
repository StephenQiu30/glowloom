package aigateway

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// RegisterCreation replaces the workbench's old quote/confirmation/query boundary.
func (c Creation) RegisterCreation(g *gin.RouterGroup) {
	g.GET("/operations/:id/text", c.downloadText)
	g.POST("/projects/:pid/free-operations", func(ctx *gin.Context) {
		p, ok := httpapi.PathUUID(ctx, "pid")
		if !ok {
			return
		}
		var in operationhttp.QuoteItemRequest
		if !httpapi.Decode(ctx, &in) {
			return
		}
		out, err := c.Quote(ctx.Request.Context(), identityhttp.Principal(ctx), p, ctx.GetHeader("Idempotency-Key"), in)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		ctx.JSON(201, operationhttp.NewQuoteResponse(out))
	})
	g.POST("/operations/:id/confirm", func(ctx *gin.Context) {
		id, ok := httpapi.PathUUID(ctx, "id")
		if !ok {
			return
		}
		var in struct {
			ProjectID uuid.UUID `json:"project_id"`
		}
		if !httpapi.Decode(ctx, &in) {
			return
		}
		out, err := c.Confirm(ctx.Request.Context(), identityhttp.Principal(ctx), in.ProjectID, id, ctx.GetHeader("Idempotency-Key"))
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		wallet, e := c.store(c.Runtime.DB, identityhttp.Principal(ctx)).Wallet(ctx.Request.Context(), out.Scope)
		if e != nil {
			creationProblem(ctx, e)
			return
		}
		available, _ := wallet["available"].(int64)
		ctx.JSON(200, gin.H{"reservation_id": out.ID, "available_micros": available * 100000, "budget_revision": 1})
	})
	g.GET("/operations/:id", func(ctx *gin.Context) {
		id, ok := httpapi.PathUUID(ctx, "id")
		if !ok {
			return
		}
		p, err := uuid.Parse(ctx.Query("project_id"))
		if err != nil {
			creationProblem(ctx, domain.ErrInvalid)
			return
		}
		out, err := c.Task(ctx.Request.Context(), identityhttp.Principal(ctx), p, id)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		ctx.Header("Cache-Control", "no-store")
		ctx.JSON(200, out)
	})
	g.GET("/projects/:pid/operations", func(ctx *gin.Context) {
		p, ok := httpapi.PathUUID(ctx, "pid")
		if !ok {
			return
		}
		out, err := c.List(ctx.Request.Context(), identityhttp.Principal(ctx), p)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		ctx.JSON(200, gin.H{"items": out, "next_cursor": nil})
	})
	g.POST("/projects/:pid/generation-estimate", func(ctx *gin.Context) {
		p, ok := httpapi.PathUUID(ctx, "pid")
		if !ok {
			return
		}
		var in struct {
			Draft promptdomain.Draft `json:"draft"`
			Force bool               `json:"force_regenerate"`
		}
		if !httpapi.Decode(ctx, &in) {
			return
		}
		out, err := c.Estimate(ctx.Request.Context(), identityhttp.Principal(ctx), p, in.Draft)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		ctx.JSON(200, out)
	})
	for _, action := range []string{"cancel", "recover"} {
		g.POST("/operations/:id/"+action, func(ctx *gin.Context) {
			id, ok := httpapi.PathUUID(ctx, "id")
			if !ok {
				return
			}
			var in struct {
				ProjectID uuid.UUID `json:"project_id"`
			}
			if !httpapi.Decode(ctx, &in) {
				return
			}
			out, err := c.Control(ctx.Request.Context(), identityhttp.Principal(ctx), in.ProjectID, id, "operation", action, ctx.GetHeader("Idempotency-Key"))
			if err != nil {
				creationProblem(ctx, err)
				return
			}
			ctx.JSON(202, out)
		})
	}
	c.registerBatches(g)
	operationhttp.NewSubmissionHandler(c).Register(g)
}

// WriteCreationProblem writes the public error response for a creation command.
func WriteCreationProblem(c *gin.Context, err error) {
	creationProblem(c, err)
}

func creationProblem(c *gin.Context, err error) {
	status, code := 503, "dependency_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, promptdomain.ErrInvalidDraft):
		status, code = 422, "invalid_request"
	case errors.Is(err, domain.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "state_conflict"
	case errors.Is(err, domain.ErrFunds):
		status, code = 409, "credits_insufficient"
	case errors.Is(err, domain.ErrBudget):
		status, code = 409, "budget_insufficient"
	case errors.Is(err, domain.ErrUnavailable):
		status, code = 409, "model_unavailable"
	case errors.Is(err, domain.ErrUnknown):
		status, code = 409, "consumption_pending"
	case errors.Is(err, promptdomain.ErrInputStale), errors.Is(err, operation.ErrQuoteSourceStale):
		status, code = 409, "canvas_revision_conflict"
	case errors.Is(err, operation.ErrQuoteStale):
		status, code = 409, "quote_stale"
	case errors.Is(err, operation.ErrSubmissionUnresolved):
		status, code = 409, "submission_unresolved"
	case errors.Is(err, operation.ErrSubmissionEpoch):
		status, code = 409, "submission_epoch_conflict"
	case errors.Is(err, operation.ErrSubmissionAccepted):
		status, code = 409, "submission_already_accepted"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
