package http

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type SelfProfileHandler struct{ profiles *application.SelfProfile }

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
func (h *SelfProfileHandler) read(c *gin.Context) {
	p, err := h.profiles.Read(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, selfProfileView(p))
}
func (h *SelfProfileHandler) update(c *gin.Context) {
	var request struct {
		ExpectedRevision string          `json:"expected_revision"`
		DisplayName      *string         `json:"display_name"`
		Avatar           json.RawMessage `json:"avatar_asset_id"`
	}
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

type PreferencesView struct {
	Theme        string `json:"theme"`
	PromptSuffix string `json:"prompt_suffix"`
	Revision     string `json:"revision"`
}

func (h *SelfProfileHandler) preferences(c *gin.Context) {
	p, err := h.profiles.ReadPreferences(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, PreferencesView{p.Theme, p.PromptSuffix, strconv.FormatInt(p.Revision, 10)})
}
func (h *SelfProfileHandler) updatePreferences(c *gin.Context) {
	var request struct {
		ExpectedRevision string  `json:"expected_revision"`
		Theme            *string `json:"theme"`
		Suffix           *string `json:"prompt_suffix"`
	}
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
