package http

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// ProfileUpdateRequest carries a versioned self profile patch.
type ProfileUpdateRequest struct {
	ExpectedRevision string          `json:"expected_revision" binding:"required"`
	DisplayName      *string         `json:"display_name" binding:"required"`
	Avatar           json.RawMessage `json:"avatar_asset_id" swaggertype:"string" extensions:"x-nullable"`
}

// PreferencesUpdateRequest carries a versioned preference patch.
type PreferencesUpdateRequest struct {
	ExpectedRevision string  `json:"expected_revision" binding:"required"`
	Theme            *string `json:"theme" binding:"required"`
	Suffix           *string `json:"prompt_suffix" binding:"required"`
}

// SelfProfileHandler adapts authenticated self-profile and preference requests.
type SelfProfileHandler struct{ profiles *application.SelfProfile }

// NewSelfProfileHandler constructs SelfProfileHandler with explicit caller-supplied dependencies.
func NewSelfProfileHandler(profiles *application.SelfProfile) *SelfProfileHandler {
	return &SelfProfileHandler{profiles: profiles}
}

// Register requires the email session middleware on the supplied group.
func (h *SelfProfileHandler) Register(group *gin.RouterGroup) {
	group.GET("/me", h.read)
	group.PATCH("/me", h.update)
	group.GET("/me/preferences", h.preferences)
	group.PATCH("/me/preferences", h.updatePreferences)
}
func selfProfileView(p application.Profile) MeView {
	capabilities := []string{"profile:write"}
	if p.PlatformRole == "platform_admin" {
		capabilities = append(capabilities, "platform:manage")
	}
	return MeView{ID: p.ID, Email: p.Email, EmailVerifiedAt: p.EmailVerifiedAt, DisplayName: p.DisplayName, AvatarAssetID: p.AvatarID, PlatformRole: p.PlatformRole, Status: string(p.Status), Revision: strconv.FormatInt(p.Revision, 10), Capabilities: capabilities}
}

// @Summary getMe
// @ID getMe
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} MeView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me [get]
func (h *SelfProfileHandler) read(c *gin.Context) {
	p, err := h.profiles.Read(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, selfProfileView(p))
}

// @Summary updateMe
// @ID updateMe
// @Tags identity
// @Accept json
// @Produce json
// @Param body body ProfileUpdateRequest true "Request"
// @Success 200 {object} MeView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me [patch]
func (h *SelfProfileHandler) update(c *gin.Context) {
	var request ProfileUpdateRequest
	if !httpapi.Decode(c, &request) {
		return
	}
	revision, err := strconv.ParseInt(request.ExpectedRevision, 10, 64)
	if err != nil || revision < 1 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	input := application.ProfileInput{ExpectedRevision: revision, DisplayName: request.DisplayName}
	if request.Avatar != nil {
		if string(request.Avatar) == "null" {
			input.ClearAvatar = true
		} else {
			var raw string
			if json.Unmarshal(request.Avatar, &raw) != nil {
				httpapi.WriteProblem(c, 422, "invalid_request", nil)
				return
			}
			id, err := uuid.Parse(raw)
			if err != nil || id == uuid.Nil {
				httpapi.WriteProblem(c, 422, "invalid_request", nil)
				return
			}
			input.AvatarID = &id
		}
	}
	p, err := h.profiles.Update(c.Request.Context(), Principal(c), input)
	if errors.Is(err, application.ErrAvatarUnavailable) {
		httpapi.WriteProblem(c, 404, "not_found", nil)
		return
	}
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, selfProfileView(p))
}

// PreferencesView returns account preferences and their concurrency version.
type PreferencesView struct {
	Theme        string `json:"theme" binding:"required"`
	PromptSuffix string `json:"prompt_suffix" binding:"required"`
	Revision     string `json:"revision" binding:"required"`
}

// @Summary getMyPreferences
// @ID getMyPreferences
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} PreferencesView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/preferences [get]
func (h *SelfProfileHandler) preferences(c *gin.Context) {
	p, err := h.profiles.ReadPreferences(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, PreferencesView{p.Theme, p.PromptSuffix, strconv.FormatInt(p.Revision, 10)})
}

// @Summary updateMyPreferences
// @ID updateMyPreferences
// @Tags identity
// @Accept json
// @Produce json
// @Param body body PreferencesUpdateRequest true "Request"
// @Success 200 {object} PreferencesView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/preferences [patch]
func (h *SelfProfileHandler) updatePreferences(c *gin.Context) {
	var request PreferencesUpdateRequest
	if !httpapi.Decode(c, &request) {
		return
	}
	expected, err := strconv.ParseInt(request.ExpectedRevision, 10, 64)
	if err != nil || expected < 1 || (request.Theme == nil && request.Suffix == nil) {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	p, err := h.profiles.ReadPreferences(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	if request.Theme != nil {
		p.Theme = *request.Theme
	}
	if request.Suffix != nil {
		p.PromptSuffix = *request.Suffix
	}
	revision, err := h.profiles.UpdatePreferences(c.Request.Context(), Principal(c), expected, p.Theme, p.PromptSuffix)
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, PreferencesView{p.Theme, p.PromptSuffix, strconv.FormatInt(revision, 10)})
}
