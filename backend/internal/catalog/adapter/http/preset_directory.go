package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type PresetDirectoryHandler struct {
	presets *application.ProviderPresets
	read    application.ReadModelIDs
}

func NewPresetDirectoryHandler(presets *application.ProviderPresets, read application.ReadModelIDs) *PresetDirectoryHandler {
	return &PresetDirectoryHandler{presets: presets, read: read}
}

func (h *PresetDirectoryHandler) Register(group *gin.RouterGroup) {
	group.POST("/admin/provider-presets/:id/model-directory", h.Read)
}

// Read reads model IDs using a temporary credential without saving it.
// @Summary 读取新连接的模型目录
// @ID readPresetModelDirectory
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "预设ID"
// @Param body body application.PresetDirectoryInput true "固定预设版本和临时凭据"
// @Success 200 {object} application.PresetDirectoryResult
// @Router /api/admin/provider-presets/{id}/model-directory [post]
func (h *PresetDirectoryHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.PresetDirectoryInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.presets.ReadDirectory(c.Request.Context(), identityhttp.Principal(c), id, input, h.read)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, result)
	case errors.Is(err, application.ErrModelDirectoryUnsupported):
		httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "model_directory_unsupported", nil)
	case errors.Is(err, application.ErrModelDirectoryCredential):
		httpapi.WriteProblem(c, http.StatusUnprocessableEntity, "model_directory_credential_rejected", nil)
	case errors.Is(err, application.ErrModelDirectoryRateLimited):
		httpapi.WriteProblem(c, http.StatusTooManyRequests, "model_directory_rate_limited", nil)
	case errors.Is(err, application.ErrModelDirectoryUnavailable):
		httpapi.WriteProblem(c, http.StatusBadGateway, "model_directory_unavailable", nil)
	default:
		presetError(c, err)
	}
}
