package http

import (
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// SpacePageView returns the available creation scopes.
type SpacePageView struct {
	Items []SpaceView `json:"items" binding:"required"`
}

// TeamMemberPageView returns a bounded, authorized member directory.
type TeamMemberPageView struct {
	Items      []TeamMemberView `json:"items" binding:"required"`
	NextCursor *string          `json:"next_cursor" binding:"required"`
	AsOf       time.Time        `json:"as_of" binding:"required"`
}

// TeamCreateRequest is the team creation input.
type TeamCreateRequest struct {
	Name     string     `json:"name" binding:"required"`
	AvatarID *uuid.UUID `json:"avatar_asset_id"`
}

// TeamRenameRequest is a versioned name change.
type TeamRenameRequest struct {
	ExpectedRevision string `json:"expected_revision" binding:"required"`
	Name             string `json:"name" binding:"required"`
}

// TeamRoleRequest is a versioned member role change.
type TeamRoleRequest struct {
	TeamMutation
	Role string `json:"role" binding:"required"`
}

// TeamLeaveRequest is a versioned departure.
type TeamLeaveRequest struct {
	ExpectedRevision string `json:"expected_revision" binding:"required"`
}

// TeamHandler exposes current-member governance without physical storage fields.
type TeamHandler struct {
	teams     *application.Teams
	directory *application.TeamDirectory
}

// NewTeamHandler injects team application services.
func NewTeamHandler(teams *application.Teams, directory *application.TeamDirectory) *TeamHandler {
	return &TeamHandler{teams: teams, directory: directory}
}

// Register requires authenticated email sessions and Origin/CSRF middleware.
func (h *TeamHandler) Register(group *gin.RouterGroup) {
	group.GET("/me/spaces", h.spaces)
	group.POST("/teams", h.create)
	group.GET("/teams/:id", h.read)
	group.PATCH("/teams/:id", h.rename)
	group.GET("/teams/:id/members", h.members)
	group.GET("/teams/:id/members/:uid", h.member)
	group.PATCH("/teams/:id/members/:uid/role", h.role)
	group.POST("/teams/:id/members/:uid/suspend", h.suspendMember)
	group.POST("/teams/:id/members/:uid/restore", h.restoreMember)
	group.POST("/teams/:id/members/:uid/remove", h.removeMember)
	group.POST("/teams/:id/leave", h.leave)
}

// TeamView projects member-visible team metadata and current capabilities.
type TeamView struct {
	ID                 uuid.UUID  `json:"id" binding:"required"`
	Name               string     `json:"name" binding:"required"`
	AvatarID           *uuid.UUID `json:"avatar_asset_id"`
	Status             string     `json:"status" binding:"required"`
	GovernanceBlocked  bool       `json:"governance_blocked" binding:"required"`
	Revision           string     `json:"revision" binding:"required"`
	MembershipRevision string     `json:"membership_revision" binding:"required"`
	Role               string     `json:"role" binding:"required"`
	MemberCount        int64      `json:"member_count" binding:"required"`
	InvitationEnabled  bool       `json:"invitation_enabled" binding:"required"`
	Capabilities       []string   `json:"capabilities" binding:"required"`
}

// SpaceView contains only user-facing scope identity and access metadata.
type SpaceView struct {
	ID       uuid.UUID  `json:"id" binding:"required"`
	Kind     string     `json:"kind" binding:"required"`
	TeamID   *uuid.UUID `json:"team_id" binding:"required"`
	Name     string     `json:"name" binding:"required"`
	Role     string     `json:"role" binding:"required"`
	Writable bool       `json:"writable" binding:"required"`
}

// TeamMemberView omits governance fields for ordinary member directories.
type TeamMemberView struct {
	UserID             uuid.UUID  `json:"user_id" binding:"required"`
	DisplayName        string     `json:"display_name" binding:"required"`
	AvatarID           *uuid.UUID `json:"avatar_asset_id" binding:"required"`
	Role               string     `json:"role" binding:"required"`
	Email              *string    `json:"email,omitempty"`
	Status             *string    `json:"status,omitempty"`
	Source             *string    `json:"source,omitempty"`
	Revision           *string    `json:"revision,omitempty"`
	JoinedAt           *time.Time `json:"joined_at,omitempty"`
	AccessAvailable    *bool      `json:"access_available,omitempty"`
	LastTeamActivityAt *time.Time `json:"last_team_activity_at,omitempty"`
}

// TeamMutation carries an optimistic concurrency version and governance reason.
type TeamMutation struct {
	ExpectedRevision string `json:"expected_revision" binding:"required"`
	Reason           string `json:"reason" binding:"required"`
}

func teamView(team application.Team) TeamView {
	caps := []string{"team:read", "members:read"}
	if team.Status == "active" && !team.GovernanceBlocked {
		caps = append(caps, "content:write")
		if team.ActorRole == "team_admin" {
			caps = append(caps, "team:manage", "members:manage")
		}
	}
	return TeamView{AvatarID: team.AvatarID, ID: team.ID, Name: team.Name, Status: team.Status, GovernanceBlocked: team.GovernanceBlocked, Revision: strconv.FormatInt(team.Revision, 10), MembershipRevision: strconv.FormatInt(team.MemberRevision, 10), Role: team.ActorRole, MemberCount: team.MemberCount, InvitationEnabled: team.InvitationEnabled, Capabilities: caps}
}
func teamProblem(c *gin.Context, err error) {
	status, code := 500, "internal_error"
	switch {
	case errors.Is(err, identity.ErrSessionInvalid):
		status, code = 401, "unauthenticated"
	case errors.Is(err, identity.ErrReauthenticationRequired):
		status, code = 403, "reauthentication_required"
	case errors.Is(err, identity.ErrForbidden), errors.Is(err, application.ErrTeamForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, application.ErrScopeNotFound), errors.Is(err, identity.ErrAvatarUnavailable):
		status, code = 404, "not_found"
	case errors.Is(err, application.ErrInvalidTeam), errors.Is(err, application.ErrInvalidMemberFilter):
		status, code = 422, "invalid_request"
	case errors.Is(err, application.ErrTeamCreationClosed):
		status, code = 403, "team_creation_closed"
	case errors.Is(err, application.ErrTeamCapacity):
		status, code = 409, "team_capacity_exceeded"
	case errors.Is(err, application.ErrTeamRevisionConflict):
		status, code = 409, "revision_conflict"
	case errors.Is(err, application.ErrTeamStateConflict):
		status, code = 409, "state_conflict"
	case errors.Is(err, application.ErrLastTeamAdministrator):
		status, code = 409, "last_team_administrator"
	case errors.Is(err, application.ErrIdempotencyConflict):
		status, code = 409, "idempotency_conflict"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
func teamRevision(c *gin.Context, value string) (int64, bool) {
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 1 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return 0, false
	}
	return revision, true
}

// @Summary listMySpaces
// @ID listMySpaces
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} SpacePageView
// @Failure 403 {object} httpapi.Problem
// @Router /api/me/spaces [get]
func (h *TeamHandler) spaces(c *gin.Context) {
	values, err := h.teams.Spaces(c.Request.Context(), identityhttp.Principal(c))
	if err != nil {
		teamProblem(c, err)
		return
	}
	items := make([]SpaceView, 0, len(values))
	for _, v := range values {
		items = append(items, SpaceView{v.ID, v.Kind, v.TeamID, v.Name, v.Role, v.Writable})
	}
	c.JSON(200, SpacePageView{Items: items})
}

// @Summary createTeam
// @ID createTeam
// @Tags identity
// @Accept json
// @Produce json
// @Param body body TeamCreateRequest true "Request"
// @Success 201 {object} TeamView
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams [post]
func (h *TeamHandler) create(c *gin.Context) {
	var input TeamCreateRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	team, err := h.teams.CreateWithAvatar(c.Request.Context(), identityhttp.Principal(c), key, input.Name, input.AvatarID)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(201, teamView(team))
}

// @Summary getTeam
// @ID getTeam
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Success 200 {object} TeamView
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id} [get]
func (h *TeamHandler) read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	team, err := h.teams.Read(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(200, teamView(team))
}

// @Summary renameTeam
// @ID renameTeam
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param body body TeamProfileRequest true "Request"
// @Success 200 {object} TeamView
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id} [patch]
func (h *TeamHandler) rename(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body TeamProfileRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	input, err := body.input()
	if err != nil {
		teamProblem(c, err)
		return
	}
	team, err := h.teams.UpdateProfile(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(200, teamView(team))
}

// @Summary listTeamMembers
// @ID listTeamMembers
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param limit query int false "Filter"
// @Param cursor query string false "Filter"
// @Param search query string false "Filter"
// @Param role query string false "Filter"
// @Param status query string false "Filter"
// @Success 200 {object} TeamMemberPageView
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/members [get]
func (h *TeamHandler) members(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	filter, err := parseMemberFilter(c.Request.URL.RawQuery)
	if err != nil {
		teamProblem(c, err)
		return
	}
	page, err := h.directory.List(c.Request.Context(), identityhttp.Principal(c), id, filter)
	if err != nil {
		teamProblem(c, err)
		return
	}
	items := make([]TeamMemberView, 0, len(page.Items))
	for _, v := range page.Items {
		items = append(items, teamMemberView(v))
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, TeamMemberPageView{Items: items, NextCursor: page.NextCursor, AsOf: page.AsOf})
}
func teamMemberView(v application.TeamMember) TeamMemberView {
	row := TeamMemberView{UserID: v.UserID, DisplayName: v.DisplayName, AvatarID: v.AvatarID, Role: v.Role, Email: v.Email, Status: v.Status, Source: v.Source, AccessAvailable: v.AccessAvailable, LastTeamActivityAt: v.LastTeamActivityAt}
	if v.Revision != nil {
		revision := strconv.FormatInt(*v.Revision, 10)
		row.Revision = &revision
		row.JoinedAt = &v.JoinedAt
	}
	return row
}

// @Summary getTeamMember
// @ID getTeamMember
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param uid path string true "Resource UUID"
// @Success 200 {object} TeamMemberView
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/members/{uid} [get]
func (h *TeamHandler) member(c *gin.Context) {
	team, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	user, ok := httpapi.PathUUID(c, "uid")
	if !ok {
		return
	}
	value, err := h.directory.Detail(c.Request.Context(), identityhttp.Principal(c), team, user)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, teamMemberView(value))
}
func parseMemberFilter(raw string) (application.MemberFilter, error) {
	f := application.MemberFilter{}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return f, application.ErrInvalidMemberFilter
	}
	for key, items := range values {
		if len(items) != 1 {
			return f, application.ErrInvalidMemberFilter
		}
		value := items[0]
		switch key {
		case "search":
			f.Search = value
		case "role":
			f.Role = value
		case "status":
			f.Status = value
		case "cursor":
			f.Cursor = value
		case "limit":
			f.Limit, err = strconv.Atoi(value)
			if err != nil || f.Limit < 1 {
				return f, application.ErrInvalidMemberFilter
			}
		case "from", "to":
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return f, application.ErrInvalidMemberFilter
			}
			if key == "from" {
				f.From = &parsed
			} else {
				f.To = &parsed
			}
		default:
			return f, application.ErrInvalidMemberFilter
		}
	}
	return f, nil
}
func (h *TeamHandler) memberAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		team, ok := httpapi.PathUUID(c, "id")
		if !ok {
			return
		}
		user, ok := httpapi.PathUUID(c, "uid")
		if !ok {
			return
		}
		var input TeamMutation
		if !httpapi.Decode(c, &input) {
			return
		}
		revision, ok := teamRevision(c, input.ExpectedRevision)
		if !ok {
			return
		}
		if err := h.teams.ChangeMember(c.Request.Context(), identityhttp.Principal(c), team, user, revision, action, input.Reason); err != nil {
			teamProblem(c, err)
			return
		}
		c.Status(204)
	}
}

// @Summary changeTeamMemberRole
// @ID changeTeamMemberRole
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param uid path string true "Resource UUID"
// @Param body body TeamRoleRequest true "Request"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/members/{uid}/role [patch]
func (h *TeamHandler) role(c *gin.Context) {
	team, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	user, ok := httpapi.PathUUID(c, "uid")
	if !ok {
		return
	}
	var input TeamRoleRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	revision, ok := teamRevision(c, input.ExpectedRevision)
	if !ok {
		return
	}
	if err := h.teams.ChangeRole(c.Request.Context(), identityhttp.Principal(c), team, user, revision, input.Role, input.Reason); err != nil {
		teamProblem(c, err)
		return
	}
	c.Status(204)
}

// @Summary leaveTeam
// @ID leaveTeam
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param body body TeamLeaveRequest true "Request"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/leave [post]
func (h *TeamHandler) leave(c *gin.Context) {
	team, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input TeamLeaveRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	revision, ok := teamRevision(c, input.ExpectedRevision)
	if !ok {
		return
	}
	if err := h.teams.Leave(c.Request.Context(), identityhttp.Principal(c), team, revision); err != nil {
		teamProblem(c, err)
		return
	}
	c.Status(204)
}

// @Summary suspendTeamMember
// @ID suspendTeamMember
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param uid path string true "Resource UUID"
// @Param body body TeamMutation true "Request"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/members/{uid}/suspend [post]
func (h *TeamHandler) suspendMember(c *gin.Context) { h.memberAction("suspend")(c) }

// @Summary restoreTeamMember
// @ID restoreTeamMember
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param uid path string true "Resource UUID"
// @Param body body TeamMutation true "Request"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/members/{uid}/restore [post]
func (h *TeamHandler) restoreMember(c *gin.Context) { h.memberAction("restore")(c) }

// @Summary removeTeamMember
// @ID removeTeamMember
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Param uid path string true "Resource UUID"
// @Param body body TeamMutation true "Request"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Router /api/teams/{id}/members/{uid}/remove [post]
func (h *TeamHandler) removeMember(c *gin.Context) { h.memberAction("remove")(c) }
