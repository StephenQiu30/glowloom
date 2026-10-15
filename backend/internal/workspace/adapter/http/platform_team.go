package http

import (
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamProfileRequest preserves omission versus explicit avatar removal.
type TeamProfileRequest struct {
	ExpectedRevision int64           `json:"expected_revision,string" swaggertype:"string"`
	Name             *string         `json:"name"`
	AvatarAssetID    json.RawMessage `json:"avatar_asset_id" swaggertype:"string" extensions:"x-nullable"`
	Reason           string          `json:"reason"`
}

func (in TeamProfileRequest) input() (application.TeamProfileInput, error) {
	out := application.TeamProfileInput{ExpectedRevision: in.ExpectedRevision, Name: in.Name, Reason: in.Reason}
	if len(in.AvatarAssetID) > 0 {
		if string(in.AvatarAssetID) == "null" {
			out.ClearAvatar = true
		} else {
			var id uuid.UUID
			if json.Unmarshal(in.AvatarAssetID, &id) != nil || id == uuid.Nil {
				return out, application.ErrInvalidTeam
			}
			out.AvatarID = &id
		}
	}
	return out, nil
}

// PlatformTeamHandler exposes metadata governance without creation scope access.
type PlatformTeamHandler struct{ service *application.PlatformTeams }

// NewPlatformTeamHandler injects platform team use cases.
func NewPlatformTeamHandler(service *application.PlatformTeams) *PlatformTeamHandler {
	return &PlatformTeamHandler{service: service}
}

// Register requires current authenticated session and Origin/CSRF middleware.
func (h *PlatformTeamHandler) Register(g *gin.RouterGroup) {
	g.GET("/platform/teams", h.list)
	g.GET("/platform/teams/:id", h.read)
	g.PATCH("/platform/teams/:id", h.update)
	g.POST("/platform/teams/:id/suspend", h.suspend)
	g.POST("/platform/teams/:id/restore", h.restore)
}
func platformTeamFilter(raw string) (application.PlatformTeamFilter, error) {
	f := application.PlatformTeamFilter{}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, application.ErrInvalidTeam
	}
	for k, v := range q {
		if len(v) != 1 || v[0] == "" {
			return f, application.ErrInvalidTeam
		}
		switch k {
		case "id", "administrator_id":
			value, parseErr := uuid.Parse(v[0])
			if parseErr != nil || value == uuid.Nil {
				return f, application.ErrInvalidTeam
			}
			if k == "id" {
				f.ID = &value
			} else {
				f.AdministratorID = &value
			}
		case "created_from", "created_to":
			value, parseErr := time.Parse(time.RFC3339Nano, v[0])
			if parseErr != nil || value.IsZero() {
				return f, application.ErrInvalidTeam
			}
			value = value.UTC()
			if k == "created_from" {
				f.CreatedFrom = &value
			} else {
				f.CreatedTo = &value
			}
		case "search":
			f.Search = v[0]
		case "status":
			f.Status = v[0]
		case "cursor":
			f.Cursor = v[0]
		case "governance_blocked":
			if v[0] != "true" && v[0] != "false" {
				return f, application.ErrInvalidTeam
			}
			value := v[0] == "true"
			f.GovernanceBlocked = &value
		case "limit":
			f.Limit, err = strconv.Atoi(v[0])
			if err != nil || f.Limit < 1 || f.Limit > 100 {
				return f, application.ErrInvalidTeam
			}
		default:
			return f, application.ErrInvalidTeam
		}
	}
	if f.CreatedFrom != nil && f.CreatedTo != nil && !f.CreatedFrom.Before(*f.CreatedTo) {
		return f, application.ErrInvalidTeam
	}
	return f, nil
}

// @Summary List platform team metadata
// @Tags identity
// @ID platformTeams
// @Produce json
// @Param cursor query string false "Signed cursor"
// @Param limit query int false "Page size"
// @Param search query string false "Name contains"
// @Param id query string false "Exact team UUID"
// @Param administrator_id query string false "Current eligible team administrator UUID"
// @Param created_from query string false "Inclusive creation time (RFC3339)"
// @Param created_to query string false "Exclusive creation time (RFC3339)"
// @Param status query string false "Team status"
// @Param governance_blocked query boolean false "Governance freeze"
// @Success 200 {object} application.PlatformTeamPage
// @Router /api/platform/teams [get]
func (h *PlatformTeamHandler) list(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := platformTeamFilter(c.Request.URL.RawQuery)
	if err != nil {
		teamProblem(c, err)
		return
	}
	out, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), f)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary Read platform team governance metadata
// @Tags identity
// @ID platformTeam
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} application.PlatformTeam
// @Router /api/platform/teams/{id} [get]
func (h *PlatformTeamHandler) read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		teamProblem(c, application.ErrInvalidTeam)
		return
	}
	out, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary Update platform team governance profile
// @Tags identity
// @ID updatePlatformTeam
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param request body TeamProfileRequest true "Profile mutation"
// @Success 200 {object} application.PlatformTeam
// @Router /api/platform/teams/{id} [patch]
func (h *PlatformTeamHandler) update(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body TeamProfileRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	in, err := body.input()
	if err != nil {
		teamProblem(c, err)
		return
	}
	out, err := h.service.Update(c.Request.Context(), identityhttp.Principal(c), id, in)
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// @Summary Suspend platform team
// @Tags identity
// @ID suspendPlatformTeam
// @Accept json
// @Param id path string true "Team ID"
// @Param request body TeamLifecycleRequest true "Version and reason"
// @Success 204
// @Router /api/platform/teams/{id}/suspend [post]
func (h *PlatformTeamHandler) suspend(c *gin.Context) { h.lifecycle(c, true) }

// @Summary Restore platform team original state
// @Tags identity
// @ID restorePlatformTeam
// @Accept json
// @Param id path string true "Team ID"
// @Param request body TeamLifecycleRequest true "Version and reason"
// @Success 204
// @Router /api/platform/teams/{id}/restore [post]
func (h *PlatformTeamHandler) restore(c *gin.Context) { h.lifecycle(c, false) }
func (h *PlatformTeamHandler) lifecycle(c *gin.Context, suspend bool) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var in TeamLifecycleRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	a := identityhttp.Principal(c)
	var err error
	if suspend {
		err = h.service.Suspend(c.Request.Context(), a, id, in.ExpectedRevision, in.Reason)
	} else {
		err = h.service.Restore(c.Request.Context(), a, id, in.ExpectedRevision, in.Reason)
	}
	if err != nil {
		teamProblem(c, err)
		return
	}
	c.Status(204)
}
