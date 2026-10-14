package http

import (
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
)

// ConfigHandler exposes explicit configuration preview and apply operations.
type ConfigHandler struct{ service *application.ProviderConfigs }

// NewConfigHandler injects the immutable snapshot use cases.
func NewConfigHandler(service *application.ProviderConfigs) *ConfigHandler {
	return &ConfigHandler{service: service}
}

// Register installs connection snapshot routes behind the session middleware.
func (h *ConfigHandler) Register(group *gin.RouterGroup) {
	group.GET("/admin/providers/:id/config", h.Read)
	group.POST("/admin/providers/:id/upgrade-preview", h.Preview)
	group.POST("/admin/providers/:id/apply-preset", h.Apply)
}

// Read returns the current editing snapshot.
// @Summary 读取连接配置快照
// @ID getProviderConfig
// @Tags settings
// @Produce json
// @Param id path string true "连接ID"
// @Success 200 {object} application.ConnectionConfig
// @Router /api/admin/providers/{id}/config [get]
func (h *ConfigHandler) Read(c *gin.Context) {
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

// Preview returns conflicts and a revision-bound content digest.
// @Summary 预览连接预设升级
// @ID previewProviderUpgrade
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "连接ID"
// @Param body body application.ConfigUpgradeInput true "原快照和目标版本"
// @Success 200 {object} application.ConfigUpgradePreview
// @Router /api/admin/providers/{id}/upgrade-preview [post]
func (h *ConfigHandler) Preview(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ConfigUpgradeInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Preview(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}

// Apply saves a new candidate snapshot while active releases stay pinned.
// @Summary 应用已审阅的连接配置
// @ID applyProviderPreset
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "连接ID"
// @Param body body application.ApplyConfigInput true "预览摘要和冲突处理"
// @Success 200 {object} application.ConnectionConfig
// @Router /api/admin/providers/{id}/apply-preset [post]
func (h *ConfigHandler) Apply(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ApplyConfigInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Apply(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}
