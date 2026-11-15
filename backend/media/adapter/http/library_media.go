package http

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// LibraryMediaHandler exposes only reauthorized leases or verified attachments.
type LibraryMediaHandler struct {
	resolve ActorResolver

	query *application.LibraryMediaQuery
}

// NewLibraryMediaHandler injects media-owned scope and private-byte verification.
func NewLibraryMediaHandler(query *application.LibraryMediaQuery, resolve ActorResolver) *LibraryMediaHandler {
	return &LibraryMediaHandler{resolve: resolve, query: query}
}

// Register keeps file effects separate from editable metadata commands.
func (h *LibraryMediaHandler) Register(group *gin.RouterGroup) {
	group.GET("/media/library/items/:item_id/preview", h.Preview)
	group.GET("/media/library/items/:item_id/download", h.Download)
}
func libraryFileScope(c *gin.Context) (domain.LibraryScope, uuid.UUID, bool) {
	scope, err := libraryScope(c.Request.URL.Query())
	if err != nil {
		writeLibraryError(c, err)
		return scope, uuid.Nil, false
	}
	for key := range c.Request.URL.Query() {
		if key != "scope" && key != "project_id" {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return scope, uuid.Nil, false
		}
	}
	item, ok := httpapi.PathUUID(c, "item_id")
	return scope, item, ok
}

// Preview provides actual image/video/audio/model private leases; no document rendering.
// @Summary 获取个人或项目素材库媒体预览
// @ID previewLibraryMediaAsset
// @Tags media
// @Produce json
// @Param item_id path string true "二进制素材条目UUID"
// @Param scope query string false "personal/project；默认personal"
// @Param project_id query string false "project scope必要UUID"
// @Success 200 {object} application.LibraryMediaPreview
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/items/{item_id}/preview [get]
func (h *LibraryMediaHandler) Preview(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	scope, item, ok := libraryFileScope(c)
	if !ok {
		return
	}
	if h == nil || h.query == nil {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	result, err := h.query.Preview(c.Request.Context(), actor, scope, item)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Download streams only original bytes verified against their current owning SHA.
// @Summary 下载素材库原件附件
// @ID downloadLibraryMediaAsset
// @Tags media
// @Produce application/octet-stream
// @Param item_id path string true "二进制素材条目UUID"
// @Param scope query string false "personal/project；默认personal"
// @Param project_id query string false "project scope必要UUID"
// @Success 200 {file} file "原始附件"
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/items/{item_id}/download [get]
func (h *LibraryMediaHandler) Download(c *gin.Context) {
	actor, actorOK := requestActor(c, h.resolve)
	if !actorOK {
		return
	}

	scope, item, ok := libraryFileScope(c)
	if !ok {
		return
	}
	if h == nil || h.query == nil {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	file, asset, err := h.query.Download(c.Request.Context(), actor, scope, item)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": asset.FileName})
	if disposition == "" {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", disposition)
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, file.Size, file.MIMEType, file.File, nil)
}
