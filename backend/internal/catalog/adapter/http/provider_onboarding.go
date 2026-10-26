package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// OnboardingHandler exposes first-time connection verification and activation.
type OnboardingHandler struct {
	service *application.ProviderOnboarding
}

// NewOnboardingHandler injects the authenticated catalog service.
func NewOnboardingHandler(service *application.ProviderOnboarding) *OnboardingHandler {
	return &OnboardingHandler{service: service}
}

// Register mounts resumable onboarding endpoints.
func (h *OnboardingHandler) Register(group *gin.RouterGroup) {
	group.GET("/admin/providers/:id/onboarding", h.Read)
	group.POST("/admin/providers/:id/onboarding/activate", h.Activate)
}

// Read returns the nonsecret saved onboarding state.
// @Summary 读取模型预设接入进度
// @ID getProviderOnboarding
// @Tags settings
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} application.ProviderOnboardingState
// @Router /api/admin/providers/{id}/onboarding [get]
func (h *OnboardingHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}

// Activate uses one successful model check to enable the selected model set.
// @Summary 验证后启用所选模型
// @ID activateProviderOnboarding
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "服务ID"
// @Param body body application.ActivateOnboardingInput true "验证记录和预期修订号"
// @Success 200 {object} application.ProviderOnboardingState
// @Router /api/admin/providers/{id}/onboarding/activate [post]
func (h *OnboardingHandler) Activate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ActivateOnboardingInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Activate(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}

// TestModel creates a charged model-access check with a stable request identity.
// @Summary 验证一个所选模型
// @ID testProviderOnboardingModel
// @Tags settings
// @Produce json
// @Param id path string true "服务ID"
// @Param credential_id path string true "凭据ID"
// @Param model_id path string true "模型ID"
// @Param Idempotency-Key header string true "请求UUID"
// @Success 202 {object} application.CredentialTestAccepted
// @Router /api/admin/providers/{id}/credentials/{credential_id}/models/{model_id}/test [post]
func (h *AdminHandler) TestModel(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	credential, ok := httpapi.PathUUID(c, "credential_id")
	if !ok {
		return
	}
	model, ok := httpapi.PathUUID(c, "model_id")
	if !ok {
		return
	}
	result, err := h.deps.CredentialTest.ExecuteModel(c.Request.Context(), identityhttp.Principal(c), id, credential, model, c.GetHeader("Idempotency-Key"))
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(202, result)
}
