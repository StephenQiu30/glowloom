package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// InvitationHandler uses authenticated email identity for preview and consumption.
type InvitationHandler struct{ service *application.Governance }

// NewInvitationHandler injects invitation use cases into authenticated HTTP routes.
func NewInvitationHandler(service *application.Governance) *InvitationHandler {
	return &InvitationHandler{service: service}
}

// RegisterPublic uses the same origin/body boundary as public email authentication.
func (h *InvitationHandler) RegisterPublic(g *gin.RouterGroup, origin string) {
	g.Group("", httpapi.Middleware(origin)).POST("/invitations/preview", h.preview)
}

// Register requires authenticated session and Origin/CSRF middleware on the supplied group.
func (h *InvitationHandler) Register(g *gin.RouterGroup) {
	g.POST("/platform/account-invitations", h.createAccount)
	g.POST("/platform/teams/:id/invitations", h.createPlatformTeam)
	g.GET("/platform/teams/:id/invitations", h.platformTeamList)
	g.POST("/platform/teams/:id/invitations/:iid/revoke", h.revokePlatformTeam)
	g.GET("/platform/account-invitations", h.accountList)
	g.POST("/platform/account-invitations/:id/resend", h.resendAccount)
	g.POST("/platform/account-invitations/:id/revoke", h.revokeAccount)
	g.POST("/teams/:id/invitations", h.createTeam)
	g.GET("/teams/:id/invitations", h.teamList)
	g.POST("/teams/:id/invitations/:iid/resend", h.resendTeam)
	g.POST("/teams/:id/invitations/:iid/revoke", h.revokeTeam)

	g.POST("/invitations/accept", h.accept)
	g.POST("/invitations/decline", h.decline)
}

// AccountInvitationRequest identifies the mailbox receiving an ordinary account invitation.
type AccountInvitationRequest struct {
	Email string `json:"email"`
}

// TeamInvitationRequest identifies the addressed email and allowed team role.
type TeamInvitationRequest struct {
	Email    string `json:"email"`
	TeamRole string `json:"team_role"`
}

// InvitationTokenRequest carries an opaque invitation proof in the request body.
type InvitationTokenRequest struct {
	Token string `json:"token"`
}

// createAccount invites a regular account without setting a password or role.
// @Summary Invite an ordinary platform account
// @Tags identity
// @ID createAccountInvitation
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Request UUID"
// @Param request body AccountInvitationRequest true "Email"
// @Success 201 {object} application.Invitation
// @Router /api/platform/account-invitations [post]
func (h *InvitationHandler) createAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	var in AccountInvitationRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.CreateAccountInvitation(c.Request.Context(), Principal(c), key, in.Email)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(201, out)
}

// createTeam invites the addressed email without modifying its global account.
// @Summary Invite a team member
// @Tags identity
// @ID createTeamInvitation
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param Idempotency-Key header string true "Request UUID"
// @Param request body TeamInvitationRequest true "Invitation"
// @Success 201 {object} application.Invitation
// @Router /api/teams/{id}/invitations [post]
func (h *InvitationHandler) createTeam(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	var in TeamInvitationRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.CreateTeamInvitation(c.Request.Context(), Principal(c), key, id, in.Email, in.TeamRole)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(201, out)
}

// preview exposes invitation metadata only to its authenticated recipient.
// @Summary Preview minimal public invitation metadata
// @Tags identity
// @ID previewInvitation
// @Accept json
// @Produce json
// @Param request body InvitationTokenRequest true "Invitation proof"
// @Success 200 {object} application.InvitationPreview
// @Router /api/invitations/preview [post]
func (h *InvitationHandler) preview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var in InvitationTokenRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.PublicInvitationPreview(c.Request.Context(), in.Token)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// accept consumes an invitation only for its verified recipient.
// @Summary AcceptInvitation
// @Tags identity
// @ID acceptInvitation
// @Accept json
// @Param request body InvitationTokenRequest true "Invitation proof"
// @Success 204
// @Router /api/invitations/accept [post]
func (h *InvitationHandler) accept(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var in InvitationTokenRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.AcceptInvitation(c.Request.Context(), Principal(c), in.Token); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// decline consumes an invitation only for its verified recipient.
// @Summary DeclineInvitation
// @Tags identity
// @ID declineInvitation
// @Accept json
// @Param request body InvitationTokenRequest true "Invitation proof"
// @Success 204
// @Router /api/invitations/decline [post]
func (h *InvitationHandler) decline(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var in InvitationTokenRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.DeclineInvitation(c.Request.Context(), Principal(c), in.Token); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// accountList returns bounded current-authority metadata.
// @Summary accountList
// @Tags identity
// @ID accountListInvitations
// @Produce json
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param status query string false "Invitation status"
// @Success 200 {object} application.InvitationListPage
// @Router /api/platform/account-invitations [get]
func (h *InvitationHandler) accountList(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseGovernanceFilter(c.Request.URL.RawQuery)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	out, err := h.service.ListInvitations(c.Request.Context(), Principal(c), nil, f)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// teamList returns bounded current-authority metadata.
// @Summary teamList
// @Tags identity
// @ID teamListInvitations
// @Produce json
// @Param id path string true "Team ID"
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param status query string false "Invitation status"
// @Success 200 {object} application.InvitationListPage
// @Router /api/teams/{id}/invitations [get]
func (h *InvitationHandler) teamList(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseGovernanceFilter(c.Request.URL.RawQuery)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	out, err := h.service.ListInvitations(c.Request.Context(), Principal(c), &id, f)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// resendAccount mutates only an account invitation through the account route.
// @Summary resendAccount
// @Tags identity
// @ID resendAccountInvitation
// @Accept json
// @Param id path string true "Invitation ID"
// @Param request body GovernanceVersion true "Expected version"
// @Success 204
// @Router /api/platform/account-invitations/{id}/resend [post]
func (h *InvitationHandler) resendAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceVersion
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.ResendInvitation(c.Request.Context(), Principal(c), id, in.ExpectedRevision, nil); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// revokeAccount mutates only an account invitation through the account route.
// @Summary revokeAccount
// @Tags identity
// @ID revokeAccountInvitation
// @Accept json
// @Param id path string true "Invitation ID"
// @Param request body GovernanceMutation true "Expected version"
// @Success 204
// @Router /api/platform/account-invitations/{id}/revoke [post]
func (h *InvitationHandler) revokeAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.RevokeInvitation(c.Request.Context(), Principal(c), id, in.ExpectedRevision, in.Reason, nil); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// resendTeam is constrained to the team in the route.
// @Summary resendTeam
// @Tags identity
// @ID resendTeamInvitation
// @Accept json
// @Param id path string true "Team ID"
// @Param iid path string true "Invitation ID"
// @Param request body GovernanceVersion true "Expected version"
// @Success 204
// @Router /api/teams/{id}/invitations/{iid}/resend [post]
func (h *InvitationHandler) resendTeam(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	iid, ok := httpapi.PathUUID(c, "iid")
	if !ok {
		return
	}
	var in GovernanceVersion
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.ResendInvitation(c.Request.Context(), Principal(c), iid, in.ExpectedRevision, &id); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// revokeTeam is constrained to the team in the route.
// @Summary revokeTeam
// @Tags identity
// @ID revokeTeamInvitation
// @Accept json
// @Param id path string true "Team ID"
// @Param iid path string true "Invitation ID"
// @Param request body GovernanceMutation true "Expected version"
// @Success 204
// @Router /api/teams/{id}/invitations/{iid}/revoke [post]
func (h *InvitationHandler) revokeTeam(c *gin.Context) {
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
	if err := h.service.RevokeInvitation(c.Request.Context(), Principal(c), iid, in.ExpectedRevision, in.Reason, &id); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}
