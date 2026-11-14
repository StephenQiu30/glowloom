package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// CustomProviderHandler exposes empty custom connections and display-only metadata.
type CustomProviderHandler struct{ service *application.CustomProviders }

// NewCustomProviderHandler injects the authorized catalog use cases.
func NewCustomProviderHandler(service *application.CustomProviders) *CustomProviderHandler {
	return &CustomProviderHandler{service: service}
}

// Register installs routes behind the session and CSRF middleware.
func (h *CustomProviderHandler) Register(group *gin.RouterGroup) {
	group.POST("/admin/providers/custom", h.Create)
	group.PATCH("/admin/providers/:id/logo", h.SetLogo)
}

// Create saves a disabled custom connection and its recoverable receipt.
// @Summary 创建自定义供应商连接
// @ID createCustomProviderConnection
// @Tags settings
// @Accept json
// @Produce json
// @Param body body application.CustomProviderInput true "供应商基本配置"
// @Success 201 {object} application.PresetConnection
// @Router /api/admin/providers/custom [post]
func (h *CustomProviderHandler) Create(c *gin.Context) {
	var input application.CustomProviderInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Create(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(201, result)
}

// SetLogo changes display metadata without altering execution or releases.
// @Summary 更新供应商 Logo
// @ID updateProviderLogo
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "供应商ID"
// @Param body body application.ProviderLogoInput true "Logo与当前修订"
// @Success 200 {object} application.CreatedProvider
// @Router /api/admin/providers/{id}/logo [patch]
func (h *CustomProviderHandler) SetLogo(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ProviderLogoInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.SetLogo(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}
