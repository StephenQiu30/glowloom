package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// PlatformPasswordResetHandler adapts audited platform recovery requests.
type PlatformPasswordResetHandler struct {
	service *application.PlatformPasswordReset
}

// NewPlatformPasswordResetHandler injects the existing recovery workflow.
func NewPlatformPasswordResetHandler(service *application.PlatformPasswordReset) *PlatformPasswordResetHandler {
	return &PlatformPasswordResetHandler{service: service}
}

// Register requires the normal authenticated Origin/CSRF middleware.
func (h *PlatformPasswordResetHandler) Register(g *gin.RouterGroup) {
	g.POST("/platform/users/:id/password-reset", h.send)
}

// @Summary Send platform requested password recovery
// @Tags identity
// @ID platformSendPasswordReset
// @Accept json
// @Param id path string true "User ID"
// @Param request body GovernanceMutation true "Version and reason"
// @Success 204
// @Router /api/platform/users/{id}/password-reset [post]
func (h *PlatformPasswordResetHandler) send(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.Send(c.Request.Context(), Principal(c), id, in.ExpectedRevision, in.Reason); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}
