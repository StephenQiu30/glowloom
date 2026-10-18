package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/presetvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// PresetHandler exposes only portable settings and safe creation receipts.
type PresetHandler struct{ service *application.ProviderPresets }

// NewPresetHandler injects the preset use cases.
func NewPresetHandler(service *application.ProviderPresets) *PresetHandler {
	return &PresetHandler{service: service}
}

// Register mounts the platform-only preset management API.
func (h *PresetHandler) Register(group *gin.RouterGroup) {
	admin := group.Group("/admin/provider-presets")
	admin.Use(func(c *gin.Context) {
		if !application.IsPlatformAdmin(identityhttp.Principal(c)) {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			c.Abort()
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	admin.GET("", h.List)
	admin.POST("", h.Create)
	admin.POST("/import-preview", h.Preview)
	admin.POST("/:id/versions", h.Version)
	admin.PATCH("/:id/state", h.State)
	admin.GET("/:id/export", h.Export)
	admin.POST("/:id/connections", h.Connect)
}

// PresetPage contains a bounded registry.
type PresetPage struct {
	Items []domain.Preset `json:"items"`
}

// PresetImportRequest preserves the exact imported JSON for strict duplicate-field checks.
type PresetImportRequest struct {
	DocumentJSON string `json:"document_json"`
}

// PresetPreview binds the portable document to its digest.
type PresetPreview struct {
	Document domain.PresetDocument `json:"document"`
	Hash     string                `json:"hash"`
}

func presetError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrReleaseUnverified):
		httpapi.WriteProblem(c, 409, "release_unverified", nil)
	case errors.Is(err, domain.ErrInvalidPreset):
		httpapi.WriteProblem(c, 422, "invalid_preset", nil)
	case errors.Is(err, application.ErrPresetNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrPresetConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrPresetUnavailable):
		httpapi.WriteProblem(c, 409, "preset_unavailable", nil)
	default:
		adminError(c, err)
	}
}

// List returns builtin and custom preset versions.
// @Summary 平台预设列表
// @ID listProviderPresets
// @Tags settings
// @Produce json
// @Success 200 {object} PresetPage
// @Router /api/admin/provider-presets [get]
func (h *PresetHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context(), identityhttp.Principal(c))
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, PresetPage{Items: items})
}

// Preview rejects keys and ambiguous fields before accepting an import.
// @Summary 预览预设导入
// @ID previewProviderPreset
// @Tags settings
// @Accept json
// @Produce json
// @Param body body PresetImportRequest true "无凭据的预设文档"
// @Success 200 {object} PresetPreview
// @Router /api/admin/provider-presets/import-preview [post]
func (h *PresetHandler) Preview(c *gin.Context) {
	var input PresetImportRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	data := []byte(input.DocumentJSON)
	doc, err := domain.ParsePreset(data)
	if err != nil {
		presetError(c, err)
		return
	}
	if err := presetvalidation.Validate(doc); err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, PresetPreview{Document: doc, Hash: doc.Digest()})
}

// Create saves the exact preview as a custom preset.
// @Summary 保存自定义预设
// @ID createProviderPreset
// @Tags settings
// @Accept json
// @Produce json
// @Param body body application.SavePresetInput true "预设与预览摘要"
// @Success 201 {object} domain.Preset
// @Router /api/admin/provider-presets [post]
func (h *PresetHandler) Create(c *gin.Context) { h.save(c, uuid.Nil) }

// Version appends a revision without editing existing connections.
// @Summary 创建预设版本
// @ID publishProviderPresetVersion
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "预设ID"
// @Param body body application.SavePresetInput true "新版本与预期修订号"
// @Success 201 {object} domain.Preset
// @Router /api/admin/provider-presets/{id}/versions [post]
func (h *PresetHandler) Version(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	h.save(c, id)
}

func (h *PresetHandler) save(c *gin.Context, id uuid.UUID) {
	var input application.SavePresetInput
	if !httpapi.Decode(c, &input) {
		return
	}
	input.ID = id
	result, err := h.service.Save(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(201, result)
}

// State archives or emergency-stops independently.
// @Summary 更新预设生命周期
// @ID updateProviderPresetState
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "预设ID"
// @Param body body application.PresetStateInput true "状态和预期修订号"
// @Success 200 {object} domain.Preset
// @Router /api/admin/provider-presets/{id}/state [patch]
func (h *PresetHandler) State(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.PresetStateInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.SetState(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}

// Export excludes account-specific and credential data by construction.
// @Summary 导出无凭据预设
// @ID exportProviderPreset
// @Tags settings
// @Produce json
// @Param id path string true "预设ID"
// @Success 200 {object} domain.PresetDocument
// @Router /api/admin/provider-presets/{id}/export [get]
func (h *PresetHandler) Export(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), identityhttp.Principal(c))
	if err != nil {
		presetError(c, err)
		return
	}
	for _, item := range items {
		if item.ID == id {
			c.Header("Content-Disposition", `attachment; filename="provider-preset.json"`)
			c.JSON(200, item.Document)
			return
		}
	}
	presetError(c, application.ErrPresetNotFound)
}

// Connect returns the original connection on a retried request.
// @Summary 从预设创建禁用连接
// @ID connectProviderPreset
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "预设ID"
// @Param body body application.ConnectPresetInput true "固定预设版本与连接名称"
// @Success 201 {object} application.PresetConnection
// @Router /api/admin/provider-presets/{id}/connections [post]
func (h *PresetHandler) Connect(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ConnectPresetInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.Connect(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(201, result)
}
