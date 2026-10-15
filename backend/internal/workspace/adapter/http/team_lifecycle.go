package http

import (
	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamLifecycleHandler exposes explicit team archive and invitation policy changes.
type TeamLifecycleHandler struct{ teams *application.Teams }

// NewTeamLifecycleHandler injects team lifecycle use cases into HTTP routes.
func NewTeamLifecycleHandler(teams *application.Teams) *TeamLifecycleHandler {
	return &TeamLifecycleHandler{teams: teams}
}

// Register requires authenticated session and Origin/CSRF middleware on the supplied group.
func (h *TeamLifecycleHandler) Register(g *gin.RouterGroup) {
	g.POST("/teams/:id/archive", h.archive)
	g.POST("/teams/:id/restore", h.restore)
	g.PATCH("/teams/:id/settings", h.settings)
}

// TeamLifecycleRequest carries an expected version and required audit reason.
type TeamLifecycleRequest struct {
	ExpectedRevision int64  `json:"expected_revision,string" swaggertype:"string"`
	Reason           string `json:"reason"`
}

// TeamInvitationSettingsRequest changes the explicit team invitation policy.
type TeamInvitationSettingsRequest struct {
	ExpectedRevision  int64 `json:"expected_revision,string" swaggertype:"string"`
	InvitationEnabled *bool `json:"invitation_enabled"`
}

// archive applies a team lifecycle transition.
// @Summary archive team
// @Tags workspace
// @ID archiveTeam
// @Accept json
// @Param id path string true "Team ID"
// @Param request body TeamLifecycleRequest true "Version and reason"
// @Success 204
// @Router /api/teams/{id}/archive [post]
func (h *TeamLifecycleHandler) archive(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in TeamLifecycleRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.teams.Archive(c.Request.Context(), identityhttp.Principal(c), id, in.ExpectedRevision, in.Reason); err != nil {
		teamProblem(c, err)
		return
	}
	c.Status(204)
}

// restore applies a team lifecycle transition.
// @Summary restore team
// @Tags workspace
// @ID restoreTeam
// @Accept json
// @Param id path string true "Team ID"
// @Param request body TeamLifecycleRequest true "Version and reason"
// @Success 204
// @Router /api/teams/{id}/restore [post]
func (h *TeamLifecycleHandler) restore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in TeamLifecycleRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.teams.Restore(c.Request.Context(), identityhttp.Principal(c), id, in.ExpectedRevision, in.Reason); err != nil {
		teamProblem(c, err)
		return
	}
	c.Status(204)
}

// settings updates the team invitation switch.
// @Summary Update team invitation policy
// @Tags workspace
// @ID updateTeamSettings
// @Accept json
// @Param id path string true "Team ID"
// @Param request body TeamInvitationSettingsRequest true "Policy"
// @Success 204
// @Router /api/teams/{id}/settings [patch]
func (h *TeamLifecycleHandler) settings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in TeamInvitationSettingsRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if in.InvitationEnabled == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if err := h.teams.SetInvitations(c.Request.Context(), identityhttp.Principal(c), id, in.ExpectedRevision, *in.InvitationEnabled); err != nil {
		teamProblem(c, err)
		return
	}
	c.Status(204)
}
