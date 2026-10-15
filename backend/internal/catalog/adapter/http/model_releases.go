package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// ReleaseHandler exposes drafts and evidence-gated publication.
type ReleaseHandler struct{ service *application.ModelReleases }

// NewReleaseHandler injects release commands.
func NewReleaseHandler(service *application.ModelReleases) *ReleaseHandler {
	return &ReleaseHandler{service: service}
}

// Register mounts the release/default management routes.
func (h *ReleaseHandler) Register(group *gin.RouterGroup) {
	group.GET("/admin/models/:id/releases", h.List)
	group.POST("/admin/models/:id/releases", h.Create)
	group.POST("/admin/model-releases/:id/activate", h.Activate)
	group.GET("/admin/capability-defaults", h.Defaults)
}

// ReleasePage contains model release history.
type ReleasePage struct {
	Items []application.ModelRelease `json:"items"`
}

// DefaultPage lists explicit capability defaults.
type DefaultPage struct {
	Items []application.CapabilityDefault `json:"items"`
}

// List returns version bindings and validation coverage.
// @Summary 模型发布历史
// @ID listModelReleases
// @Tags settings
// @Produce json
// @Param id path string true "模型ID"
// @Success 200 {object} ReleasePage
// @Router /api/admin/models/{id}/releases [get]
func (h *ReleaseHandler) List(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, ReleasePage{Items: items})
}

// Create freezes model, supplier configuration, price and cost ceiling.
// @Summary 创建待验证模型发布
// @ID createModelRelease
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "模型ID"
// @Param body body application.CreateReleaseInput true "发布绑定"
// @Success 201 {object} application.ModelRelease
// @Router /api/admin/models/{id}/releases [post]
func (h *ReleaseHandler) Create(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.CreateReleaseInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Create(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(201, result)
}

// Activate checks real evidence before switching visible model/default bindings.
// @Summary 激活已验证模型发布
// @ID activateModelRelease
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "发布ID"
// @Param body body application.ActivateReleaseInput true "预期版本及默认设置"
// @Success 200 {object} application.ModelRelease
// @Router /api/admin/model-releases/{id}/activate [post]
func (h *ReleaseHandler) Activate(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ActivateReleaseInput
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

// Defaults returns platform defaults pinned to published releases.
// @Summary 平台能力默认模型
// @ID listCapabilityDefaults
// @Tags settings
// @Produce json
// @Success 200 {object} DefaultPage
// @Router /api/admin/capability-defaults [get]
func (h *ReleaseHandler) Defaults(c *gin.Context) {
	items, err := h.service.Defaults(c.Request.Context(), identityhttp.Principal(c))
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, DefaultPage{Items: items})
}
