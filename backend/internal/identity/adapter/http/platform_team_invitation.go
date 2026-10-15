package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// PlatformTeamInvitationRequest requires an explicit audit reason for platform intervention.
type PlatformTeamInvitationRequest struct {
	Email    string `json:"email"`
	TeamRole string `json:"team_role"`
	Reason   string `json:"reason"`
}

// @Summary Invite a team member under platform governance
// @Tags identity
// @ID createPlatformTeamInvitation
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param Idempotency-Key header string true "Request UUID"
// @Param request body PlatformTeamInvitationRequest true "Invitation"
// @Success 201 {object} application.Invitation
// @Router /api/platform/teams/{id}/invitations [post]
func (h *InvitationHandler) createPlatformTeam(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	var in PlatformTeamInvitationRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	var out application.Invitation
	out, err = h.service.CreatePlatformTeamInvitation(c.Request.Context(), Principal(c), key, id, in.Email, in.TeamRole, in.Reason)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(201, out)
}

// @Summary Revoke a team invitation under platform governance
// @Tags identity
// @ID revokePlatformTeamInvitation
// @Accept json
// @Param id path string true "Team ID"
// @Param iid path string true "Invitation ID"
// @Param request body GovernanceMutation true "Version and reason"
// @Success 204
// @Router /api/platform/teams/{id}/invitations/{iid}/revoke [post]
func (h *InvitationHandler) revokePlatformTeam(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	iid, ok := httpapi.PathUUID(c, "iid")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.RevokePlatformTeamInvitation(c.Request.Context(), Principal(c), id, iid, in.ExpectedRevision, in.Reason); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}
