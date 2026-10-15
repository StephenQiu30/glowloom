package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamAvatarHandler serves previews selected only by actual team/member bindings.
type TeamAvatarHandler struct{ service *application.TeamAvatars }

// NewTeamAvatarHandler injects authorized preview use cases.
func NewTeamAvatarHandler(service *application.TeamAvatars) *TeamAvatarHandler {
	return &TeamAvatarHandler{service: service}
}

// Register requires authenticated session middleware.
func (h *TeamAvatarHandler) Register(g *gin.RouterGroup) {
	g.GET("/teams/:id/avatar", h.team)
	g.GET("/teams/:id/members/:uid/avatar", h.member)
	g.GET("/platform/teams/:id/avatar", h.platform)
}

// @Summary Preview team avatar
// @Tags identity
// @ID teamAvatar
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} application.TeamAvatarPreview
// @Router /api/teams/{id}/avatar [get]
func (h *TeamAvatarHandler) team(c *gin.Context) { h.read(c, false, false) }

// @Summary Preview directory member avatar
// @Tags identity
// @ID teamMemberAvatar
// @Produce json
// @Param id path string true "Team ID"
// @Param uid path string true "Member ID"
// @Success 200 {object} application.TeamAvatarPreview
// @Router /api/teams/{id}/members/{uid}/avatar [get]
func (h *TeamAvatarHandler) member(c *gin.Context) { h.read(c, true, false) }

// @Summary Preview platform team governance avatar
// @Tags identity
// @ID platformTeamAvatar
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} application.TeamAvatarPreview
// @Router /api/platform/teams/{id}/avatar [get]
func (h *TeamAvatarHandler) platform(c *gin.Context) { h.read(c, false, true) }
func (h *TeamAvatarHandler) read(c *gin.Context, isMember, platform bool) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" {
		teamProblem(c, application.ErrInvalidTeam)
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var member *uuid.UUID
	if isMember {
		uid, ok := httpapi.PathUUID(c, "uid")
		if !ok {
			return
		}
		member = &uid
	}
	out, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), id, member, platform)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(200, out)
}
