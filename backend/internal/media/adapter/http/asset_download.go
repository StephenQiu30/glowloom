package http

import (
	"errors"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AssetDownloadHandler exposes attachment-only image/video/audio originals.
type AssetDownloadHandler struct{ downloads *application.AssetDownloads }

// NewAssetDownloadHandler injects the original-object owner.
func NewAssetDownloadHandler(downloads *application.AssetDownloads) *AssetDownloadHandler {
	return &AssetDownloadHandler{downloads: downloads}
}

// Register installs the project original route without replacing document download.
func (h *AssetDownloadHandler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid/media/:asset_id/original", h.Download)
}

// Download streams an eligible original after current authorization and identity checks.
// @Summary 下载图片、视频或音频原件
// @ID downloadMediaOriginal
// @Tags media
// @Produce application/octet-stream
// @Param pid path string true "项目UUID"
// @Param asset_id path string true "媒体资产UUID"
// @Success 200 {file} file "原始媒体附件"
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media/{asset_id}/original [get]
func (h *AssetDownloadHandler) Download(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "asset_id")
	if !ok {
		return
	}
	file, err := h.downloads.Open(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		if errors.Is(err, application.ErrOriginalChanged) {
			httpapi.WriteProblem(c, http.StatusConflict, "media_original_changed", nil)
		} else {
			writeError(c, err)
		}
		return
	}
	defer func() { _ = file.Body.Close() }()
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.FileName})
	if disposition == "" {
		writeError(c, application.ErrUnavailable)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", disposition)
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, file.Size, file.MIMEType, file.Body, nil)
}
