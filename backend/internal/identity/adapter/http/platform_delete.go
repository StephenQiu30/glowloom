package http

import (
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// deleteUser soft-deletes an account under current platform authority.
// @Summary Delete platform account and retain resources
// @Tags identity
// @ID platformDeleteUser
// @Accept json
// @Param id path string true "Account ID"
// @Param Idempotency-Key header string true "Original command UUID"
// @Param request body GovernanceVersion true "Expected account version"
// @Success 204
// @Failure 409 {object} httpapi.Problem
// @Router /api/platform/users/{id}/delete [post]
func (h *GovernanceHandler) deleteUser(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	var in GovernanceVersion
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.ExecuteUserCommand(c.Request.Context(), Principal(c), id, in.ExpectedRevision, "", "delete", key); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}
