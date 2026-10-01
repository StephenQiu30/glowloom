// Package http adapts workspace prompt preferences to the protected public API.
package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
)

// Handler exposes personal preferences without administrator operations.
type Handler struct{ preferences *application.Preferences }

// NewHandler injects the workspace preference use cases.
func NewHandler(preferences *application.Preferences) *Handler {
	return &Handler{preferences: preferences}
}

// Register installs actor-scoped reads and guarded replacement/reset commands.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/settings/prompt-preferences", h.List)
	group.PUT("/settings/prompt-preferences/:operation", h.Save)
}

// PreferencesResponse includes all allowed baselines and the current actor's layers.
type PreferencesResponse struct {
	Items []application.Preference `json:"items"`
}

// SaveRequest modifies only the personal creative layer of an existing operation.
type SaveRequest struct {
	ExpectedRevision *int64      `json:"expected_revision" binding:"required"`
	BaseTemplateID   string      `json:"base_template_id" binding:"required"`
	Mode             domain.Mode `json:"mode" binding:"required"`
	Content          string      `json:"content"`
}

// List exposes private preferences and read-only platform constraints.
// @Summary 工作区提示词偏好
// @ID listPromptPreferences
// @Tags settings
// @Produce json
// @Success 200 {object} PreferencesResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/settings/prompt-preferences [get]
func (h *Handler) List(c *gin.Context) {
	preferences, err := h.preferences.List(c.Request.Context(), identityhttp.Principal(c))
	if err != nil {
		writePreferenceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, PreferencesResponse{Items: preferences})
}

// Save commits a guarded personal layer or restores inheritance with empty content.
// @Summary 保存或恢复工作区提示词偏好
// @ID savePromptPreference
// @Tags settings
// @Accept json
// @Produce json
// @Param operation path string true "只读目录中的操作标识"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body SaveRequest true "模式、内容、基线及读取时修订"
// @Success 200 {object} domain.Customization
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/settings/prompt-preferences/{operation} [put]
func (h *Handler) Save(c *gin.Context) {
	var body SaveRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	baseline, baselineErr := uuid.Parse(body.BaseTemplateID)
	key, keyErr := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if baselineErr != nil || baseline == uuid.Nil || keyErr != nil || key == uuid.Nil || body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	customization, err := h.preferences.Save(c.Request.Context(), identityhttp.Principal(c), application.SaveInput{
		Operation: c.Param("operation"), Mode: body.Mode, Content: body.Content, BaseTemplateID: baseline,
		ExpectedRevision: *body.ExpectedRevision, IdempotencyKey: key, RequestID: c.GetString("request_id"),
	})
	if err != nil {
		writePreferenceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, customization)
}

func writePreferenceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrBaselineConflict):
		httpapi.WriteProblem(c, 409, "baseline_changed", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, domain.ErrInvalidCustomization):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
