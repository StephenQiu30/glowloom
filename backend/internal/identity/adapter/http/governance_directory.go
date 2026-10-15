package http

import (
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func parseGovernanceFilter(raw string) (application.GovernanceFilter, error) {
	f := application.GovernanceFilter{}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, application.ErrInvalidGovernance
	}
	for k, v := range q {
		if len(v) != 1 || v[0] == "" {
			return f, application.ErrInvalidGovernance
		}
		switch k {
		case "status":
			f.Status = v[0]
		case "cursor":
			f.Cursor = v[0]
		case "limit":
			f.Limit, err = strconv.Atoi(v[0])
			if err != nil || f.Limit < 1 || f.Limit > 100 {
				return f, application.ErrInvalidGovernance
			}
		default:
			return f, application.ErrInvalidGovernance
		}
	}
	return f, nil
}

// teamProposals returns the current team's signed transfer directory.
// @Summary List team administrator transfers
// @Tags identity
// @ID teamAuthorityProposals
// @Produce json
// @Param id path string true "Team ID"
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param status query string false "Proposal status"
// @Success 200 {object} application.ProposalListPage
// @Router /api/teams/{id}/admin-transfers [get]
func (h *GovernanceHandler) teamProposals(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	f, err := parseGovernanceFilter(c.Request.URL.RawQuery)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	page, err := h.service.ListProposals(c.Request.Context(), Principal(c), &id, "team", f)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, page)
}

// @Summary List platform-governed team invitations
// @ID listPlatformTeamInvitations
// @Tags identity
// @Produce json
// @Param id path string true "Team ID"
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param status query string false "Invitation status"
// @Success 200 {object} application.InvitationListPage
// @Router /api/platform/teams/{id}/invitations [get]
func (h *InvitationHandler) platformTeamList(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	team, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	filter, err := parseGovernanceFilter(c.Request.URL.RawQuery)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	page, err := h.service.ListPlatformTeamInvitations(c.Request.Context(), Principal(c), team, filter)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, page)
}
