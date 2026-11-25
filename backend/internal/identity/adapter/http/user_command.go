package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func (h *GovernanceHandler) userCommand(c *gin.Context, id uuid.UUID, revision int64, reason, action string) error {
	raw := c.GetHeader("Idempotency-Key")
	if raw != "" {
		key, err := uuid.Parse(raw)
		if err != nil {
			return application.ErrInvalidGovernance
		}
		return h.service.ExecuteUserCommand(c.Request.Context(), Principal(c), id, revision, reason, action, key)
	}
	// Preserve existing clients; new bulk callers always supply an identity.
	switch action {
	case "disable":
		return h.service.Disable(c.Request.Context(), Principal(c), id, revision, reason)
	case "enable":
		return h.service.Enable(c.Request.Context(), Principal(c), id, revision, reason)
	default:
		return h.service.RevokeSessions(c.Request.Context(), Principal(c), id, revision, reason)
	}
}

// userCommandReceipt returns proof of an original committed account action.
// @Summary Recover account governance command
// @Tags identity
// @ID getUserGovernanceReceipt
// @Param id path string true "Account ID"
// @Param request path string true "Original request UUID"
// @Success 200 {object} application.UserCommandReceipt
// @Router /api/platform/users/{id}/commands/{request} [get]
func (h *GovernanceHandler) userCommandReceipt(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	request, ok := httpapi.PathUUID(c, "request")
	if !ok {
		return
	}
	receipt, err := h.service.UserCommandReceipt(c.Request.Context(), Principal(c), id, request)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, receipt)
}
