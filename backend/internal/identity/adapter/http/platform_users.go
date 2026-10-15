package http

import (
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type PlatformDirectoryHandler struct {
	directory *application.PlatformDirectory
}

func NewPlatformDirectoryHandler(directory *application.PlatformDirectory) *PlatformDirectoryHandler {
	return &PlatformDirectoryHandler{directory: directory}
}

// Register requires the email session middleware; persistence checks authority again.
func (h *PlatformDirectoryHandler) Register(group *gin.RouterGroup) {
	group.GET("/platform/users", h.list)
	group.GET("/platform/users/:id", h.detail)
}

type PlatformUserView struct {
	ID                 uuid.UUID  `json:"id"`
	Email              string     `json:"email"`
	EmailVerifiedAt    *time.Time `json:"email_verified_at"`
	DisplayName        string     `json:"display_name"`
	AvatarAssetID      *uuid.UUID `json:"avatar_asset_id"`
	Status             string     `json:"status"`
	PlatformRole       string     `json:"platform_role"`
	Revision           string     `json:"revision"`
	CreateTime         time.Time  `json:"create_time"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	ActiveTeamCount    int        `json:"active_team_count"`
	MustChangePassword bool       `json:"must_change_password"`
}

type PlatformUserPageView struct {
	Items      []PlatformUserView `json:"items"`
	NextCursor *string            `json:"next_cursor"`
	AsOf       time.Time          `json:"as_of"`
}

type TeamMembershipView struct {
	TeamID uuid.UUID `json:"team_id"`
	Role   string    `json:"role"`
	Status string    `json:"status"`
}

type PlatformUserDetailView struct {
	User           PlatformUserView     `json:"user"`
	Teams          []TeamMembershipView `json:"teams"`
	AllowedActions []string             `json:"allowed_actions"`
}

func platformUserView(u application.UserSummary) PlatformUserView {
	return PlatformUserView{ID: u.ID, Email: u.Email, EmailVerifiedAt: u.EmailVerifiedAt, DisplayName: u.DisplayName, AvatarAssetID: u.AvatarAssetID, Status: u.Status, PlatformRole: u.PlatformRole, Revision: strconv.FormatInt(u.Revision, 10), CreateTime: u.CreatedAt, LastLoginAt: u.LastLoginAt, ActiveTeamCount: u.ActiveTeamCount, MustChangePassword: u.MustChangePassword}
}

func parseDirectoryFilter(raw string) (application.DirectoryFilter, error) {
	f := application.DirectoryFilter{}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return f, application.ErrInvalidDirectoryFilter
	}
	for key, items := range values {
		if len(items) != 1 {
			return f, application.ErrInvalidDirectoryFilter
		}
		value := items[0]
		switch key {
		case "cursor":
			f.Cursor = value
		case "search":
			f.Search = value
		case "status":
			f.Status = value
		case "platform_role":
			f.PlatformRole = value
		case "order":
			f.Order = value
		case "limit":
			f.Limit, err = strconv.Atoi(value)
			if err != nil || f.Limit < 1 {
				return f, application.ErrInvalidDirectoryFilter
			}
		case "verified", "in_team":
			if value != "true" && value != "false" {
				return f, application.ErrInvalidDirectoryFilter
			}
			v := value == "true"
			if key == "verified" {
				f.Verified = &v
			} else {
				f.InTeam = &v
			}
		case "from", "to":
			v, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return f, application.ErrInvalidDirectoryFilter
			}
			if key == "from" {
				f.From = &v
			} else {
				f.To = &v
			}
		default:
			return f, application.ErrInvalidDirectoryFilter
		}
	}
	return f, nil
}

func directoryProblem(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidDirectoryFilter):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, domain.ErrUserNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	default:
		emailAuthProblem(c, err)
	}
}

func (h *PlatformDirectoryHandler) list(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseDirectoryFilter(c.Request.URL.RawQuery)
	if err != nil {
		directoryProblem(c, err)
		return
	}
	page, err := h.directory.List(c.Request.Context(), Principal(c), f)
	if err != nil {
		directoryProblem(c, err)
		return
	}
	items := make([]PlatformUserView, 0, len(page.Items))
	for _, u := range page.Items {
		items = append(items, platformUserView(u))
	}
	c.JSON(200, PlatformUserPageView{Items: items, NextCursor: page.NextCursor, AsOf: page.AsOf})
}

func (h *PlatformDirectoryHandler) detail(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	detail, err := h.directory.Detail(c.Request.Context(), Principal(c), id)
	if err != nil {
		directoryProblem(c, err)
		return
	}
	teams := make([]TeamMembershipView, 0, len(detail.Teams))
	for _, m := range detail.Teams {
		teams = append(teams, TeamMembershipView{TeamID: m.TeamID, Role: m.Role, Status: m.Status})
	}
	// Governance actions remain closed until atomic team freeze and grants exist.
	c.JSON(200, PlatformUserDetailView{User: platformUserView(detail.User), Teams: teams, AllowedActions: []string{}})
}
