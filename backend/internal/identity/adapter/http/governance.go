package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// GovernanceHandler exposes only explicit account-policy and proposal actions.
type GovernanceHandler struct{ service *application.Governance }

// NewGovernanceHandler injects the governance use cases into HTTP routes.
func NewGovernanceHandler(service *application.Governance) *GovernanceHandler {
	return &GovernanceHandler{service: service}
}

// Register requires authenticated session and Origin/CSRF middleware on the supplied group.
func (h *GovernanceHandler) Register(g *gin.RouterGroup) {
	g.GET("/platform/users/:id/commands/:request", h.userCommandReceipt)
	g.POST("/platform/users/:id/disable", h.disable)
	g.POST("/platform/users/:id/delete", h.deleteUser)
	g.POST("/platform/users/:id/enable", h.enable)
	g.POST("/platform/users/:id/unlock", h.unlock)
	g.POST("/platform/users/:id/revoke-admin", h.revokeAdmin)
	g.POST("/platform/users/:id/revoke-sessions", h.revokeSessions)
	g.POST("/platform/users/:id/profile-reset", h.resetProfile)
	g.GET("/platform/settings", h.settings)
	g.PATCH("/platform/settings", h.updateSettings)
	g.POST("/platform/admin-grants", h.proposeAdmin)
	g.GET("/platform/admin-grants", h.platformProposals)
	g.POST("/platform/admin-grants/:id/cancel", h.cancelProposal)
	g.GET("/me/authority-proposals", h.myProposals)
	g.POST("/me/authority-proposals/:id/accept", h.acceptProposal)
	g.POST("/me/authority-proposals/:id/decline", h.declineProposal)
	g.POST("/teams/:id/admin-transfers", h.proposeTransfer)
	g.GET("/teams/:id/admin-transfers", h.teamProposals)
	g.POST("/teams/:id/admin-transfers/:pid/cancel", h.cancelTransfer)
	g.POST("/platform/teams/:id/admin-transfers", h.proposeTakeover)
}

// GovernanceMutation carries compare-and-swap and audit context only.
type GovernanceMutation struct {
	ExpectedRevision int64  `json:"expected_revision,string" swaggertype:"string"`
	Reason           string `json:"reason"`
}

// GovernanceVersion carries the string-encoded optimistic version for a proposal action.
type GovernanceVersion struct {
	ExpectedRevision int64 `json:"expected_revision,string" swaggertype:"string"`
}

// SessionGovernanceRequest carries the account version and reason for revoking sessions.
type SessionGovernanceRequest struct {
	ExpectedRevision int64  `json:"expected_revision,string" swaggertype:"string"`
	Reason           string `json:"reason"`
}

// ProfileGovernanceRequest selects public profile reset fields with version and audit reason.
type ProfileGovernanceRequest struct {
	ExpectedRevision int64  `json:"expected_revision,string" swaggertype:"string"`
	Reason           string `json:"reason"`
	ResetDisplayName bool   `json:"reset_display_name"`
	ResetAvatar      bool   `json:"reset_avatar"`
}

// SettingsGovernanceRequest contains only the explicit platform policy allowlist.
type SettingsGovernanceRequest struct {
	ExpectedRevision    int64  `json:"expected_revision,string" swaggertype:"string"`
	Reason              string `json:"reason"`
	RegistrationEnabled *bool  `json:"registration_enabled"`
	TeamCreationEnabled *bool  `json:"team_creation_enabled"`
}

// AuthorityProposalRequest identifies the target, expected version and audit reason.
type AuthorityProposalRequest struct {
	ExpectedRevision int64     `json:"expected_revision,string" swaggertype:"string"`
	Reason           string    `json:"reason"`
	TargetUserID     uuid.UUID `json:"target_user_id"`
}

func governanceProblem(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidGovernance):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, application.ErrLastPlatformAdmin):
		httpapi.WriteProblem(c, 409, "last_platform_administrator", nil)
	case errors.Is(err, application.ErrDeletionDuties):
		httpapi.WriteProblem(c, 409, "deletion_duties", nil)
	case errors.Is(err, application.ErrGovernanceConflict), errors.Is(err, workspace.ErrTeamStateConflict), errors.Is(err, workspace.ErrTeamCapacity):
		httpapi.WriteProblem(c, 409, "governance_conflict", nil)
	case errors.Is(err, workspace.ErrTeamRevisionConflict), errors.Is(err, domain.ErrRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, workspace.ErrTeamForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, workspace.ErrScopeNotFound), errors.Is(err, domain.ErrUserNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	default:
		emailAuthProblem(c, err)
	}
}

// disable applies a platform account mutation.
// @Summary Disable account
// @Tags identity
// @ID platformDisable
// @Accept json
// @Param id path string true "Account ID"
// @Param request body GovernanceMutation true "Version and reason"
// @Success 204
// @Router /api/platform/users/{id}/disable [post]
func (h *GovernanceHandler) disable(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.userCommand(c, id, in.ExpectedRevision, in.Reason, "disable"); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// enable applies a platform account mutation.
// @Summary Enable account
// @Tags identity
// @ID platformEnable
// @Accept json
// @Param id path string true "Account ID"
// @Param request body GovernanceMutation true "Version and reason"
// @Success 204
// @Router /api/platform/users/{id}/enable [post]
func (h *GovernanceHandler) enable(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.userCommand(c, id, in.ExpectedRevision, in.Reason, "enable"); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// unlock applies a platform account mutation.
// @Summary Unlock account
// @Tags identity
// @ID platformUnlock
// @Accept json
// @Param id path string true "Account ID"
// @Param request body GovernanceMutation true "Version and reason"
// @Success 204
// @Router /api/platform/users/{id}/unlock [post]
func (h *GovernanceHandler) unlock(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.Unlock(c.Request.Context(), Principal(c), id, in.ExpectedRevision, in.Reason); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// revokeAdmin applies a platform account mutation.
// @Summary RevokeAdmin account
// @Tags identity
// @ID platformRevokeAdmin
// @Accept json
// @Param id path string true "Account ID"
// @Param request body GovernanceMutation true "Version and reason"
// @Success 204
// @Router /api/platform/users/{id}/revoke-admin [post]
func (h *GovernanceHandler) revokeAdmin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.RevokeAdmin(c.Request.Context(), Principal(c), id, in.ExpectedRevision, in.Reason); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// revokeSessions revokes current credentials without reading private resources.
// @Summary Revoke account sessions
// @Tags identity
// @ID platformRevokeSessions
// @Accept json
// @Param id path string true "Account ID"
// @Param request body SessionGovernanceRequest true "Version and policy"
// @Success 204
// @Router /api/platform/users/{id}/revoke-sessions [post]
func (h *GovernanceHandler) revokeSessions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in SessionGovernanceRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.userCommand(c, id, in.ExpectedRevision, in.Reason, "revoke_sessions"); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// resetProfile resets only public profile fields.
// @Summary Reset account public profile
// @Tags identity
// @ID platformResetProfile
// @Accept json
// @Param id path string true "Account ID"
// @Param request body ProfileGovernanceRequest true "Version and fields"
// @Success 204
// @Router /api/platform/users/{id}/profile-reset [post]
func (h *GovernanceHandler) resetProfile(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in ProfileGovernanceRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.ResetProfile(c.Request.Context(), Principal(c), id, in.ExpectedRevision, in.Reason, in.ResetDisplayName, in.ResetAvatar); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// settings reads the explicit platform policy allowlist.
// @Summary Read platform settings
// @Tags identity
// @ID platformSettings
// @Produce json
// @Success 200 {object} application.PlatformSettings
// @Router /api/platform/settings [get]
func (h *GovernanceHandler) settings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	out, err := h.service.Settings(c.Request.Context(), Principal(c))
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// updateSettings applies versioned platform policy.
// @Summary Update platform settings
// @Tags identity
// @ID updatePlatformSettings
// @Accept json
// @Produce json
// @Param request body SettingsGovernanceRequest true "Settings change"
// @Success 200 {object} application.PlatformSettings
// @Router /api/platform/settings [patch]
func (h *GovernanceHandler) updateSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var in SettingsGovernanceRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.UpdateSettings(c.Request.Context(), Principal(c), in.ExpectedRevision, in.RegistrationEnabled, in.TeamCreationEnabled, in.Reason)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// proposeAdmin records an explicit acceptance proposal.
// @Summary ProposeAdmin
// @Tags identity
// @ID proposeAdmin
// @Param Idempotency-Key header string true "Request UUID"
// @Accept json
// @Produce json
// @Param request body AuthorityProposalRequest true "Proposal"
// @Success 201 {object} application.AuthorityProposal
// @Router /api/platform/admin-grants [post]
func (h *GovernanceHandler) proposeAdmin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	var in AuthorityProposalRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.ProposeAdminWithKey(c.Request.Context(), Principal(c), key, in.TargetUserID, in.ExpectedRevision, in.Reason)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(201, out)
}

// proposeTransfer records an explicit acceptance proposal.
// @Summary ProposeTransfer
// @Tags identity
// @ID proposeTransfer
// @Param Idempotency-Key header string true "Request UUID"
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param request body AuthorityProposalRequest true "Proposal"
// @Success 201 {object} application.AuthorityProposal
// @Router /api/teams/{id}/admin-transfers [post]
func (h *GovernanceHandler) proposeTransfer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in AuthorityProposalRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.ProposeTransferWithKey(c.Request.Context(), Principal(c), key, id, in.TargetUserID, in.ExpectedRevision, in.Reason)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(201, out)
}

// proposeTakeover records an explicit acceptance proposal.
// @Summary ProposeTakeover
// @Tags identity
// @ID proposeTakeover
// @Param Idempotency-Key header string true "Request UUID"
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param request body AuthorityProposalRequest true "Proposal"
// @Success 201 {object} application.AuthorityProposal
// @Router /api/platform/teams/{id}/admin-transfers [post]
func (h *GovernanceHandler) proposeTakeover(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in AuthorityProposalRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.service.ProposeTakeoverWithKey(c.Request.Context(), Principal(c), key, id, in.TargetUserID, in.ExpectedRevision, in.Reason)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(201, out)
}

// acceptProposal updates a proposal without accepting caller-supplied authority.
// @Summary AcceptProposal
// @Tags identity
// @ID acceptProposal
// @Accept json
// @Param id path string true "Proposal ID"
// @Param request body GovernanceVersion true "Expected version"
// @Success 204
// @Router /api/me/authority-proposals/{id}/accept [post]
func (h *GovernanceHandler) acceptProposal(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceVersion
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.AcceptProposal(c.Request.Context(), Principal(c), id, in.ExpectedRevision); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// declineProposal updates a proposal without accepting caller-supplied authority.
// @Summary DeclineProposal
// @Tags identity
// @ID declineProposal
// @Accept json
// @Param id path string true "Proposal ID"
// @Param request body GovernanceVersion true "Expected version"
// @Success 204
// @Router /api/me/authority-proposals/{id}/decline [post]
func (h *GovernanceHandler) declineProposal(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceVersion
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.DeclineProposal(c.Request.Context(), Principal(c), id, in.ExpectedRevision); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// cancelProposal updates a proposal without accepting caller-supplied authority.
// @Summary CancelProposal
// @Tags identity
// @ID cancelProposal
// @Accept json
// @Param id path string true "Proposal ID"
// @Param request body GovernanceMutation true "Expected version"
// @Success 204
// @Router /api/platform/admin-grants/{id}/cancel [post]
func (h *GovernanceHandler) cancelProposal(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.CancelProposal(c.Request.Context(), Principal(c), id, in.ExpectedRevision, in.Reason); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}

// myProposals lists at most 100 currently authorized metadata rows.
// @Summary myProposals
// @Tags identity
// @ID myProposals
// @Produce json
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param status query string false "Proposal status"
// @Success 200 {object} application.ProposalListPage
// @Router /api/me/authority-proposals [get]
func (h *GovernanceHandler) myProposals(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseGovernanceFilter(c.Request.URL.RawQuery)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	out, err := h.service.ListProposals(c.Request.Context(), Principal(c), nil, "mine", f)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// platformProposals lists at most 100 currently authorized metadata rows.
// @Summary platformProposals
// @Tags identity
// @ID platformProposals
// @Produce json
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param status query string false "Proposal status"
// @Success 200 {object} application.ProposalListPage
// @Router /api/platform/admin-grants [get]
func (h *GovernanceHandler) platformProposals(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseGovernanceFilter(c.Request.URL.RawQuery)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	out, err := h.service.ListProposals(c.Request.Context(), Principal(c), nil, "platform", f)
	if err != nil {
		governanceProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// cancelTransfer binds withdrawal to the current team route.
// @Summary Cancel team authority transfer
// @Tags identity
// @ID cancelTeamTransfer
// @Accept json
// @Param id path string true "Team ID"
// @Param pid path string true "Proposal ID"
// @Param request body GovernanceMutation true "Expected version"
// @Success 204
// @Router /api/teams/{id}/admin-transfers/{pid}/cancel [post]
func (h *GovernanceHandler) cancelTransfer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var in GovernanceMutation
	if !httpapi.Decode(c, &in) {
		return
	}
	if err := h.service.CancelTransfer(c.Request.Context(), Principal(c), id, pid, in.ExpectedRevision, in.Reason); err != nil {
		governanceProblem(c, err)
		return
	}
	c.Status(204)
}
