package http

import (
	"errors"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// DocumentHandler streams only reauthorized, SHA-verified document originals.
type DocumentHandler struct{ sources *application.DocumentSources }

// NewDocumentHandler injects the media-owned original reader.
func NewDocumentHandler(sources *application.DocumentSources) *DocumentHandler {
	return &DocumentHandler{sources: sources}
}

// Register installs the attachment-only original document route.
func (h *DocumentHandler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid/media/:asset_id/download", h.Download)
}

// Download returns an original document as an attachment, never an inline preview.
// @Summary 下载正式文档原件
// @ID downloadDocumentAsset
// @Tags media
// @Produce application/octet-stream
// @Param pid path string true "项目UUID"
// @Param asset_id path string true "文档素材UUID"
// @Success 200 {file} file "原始TXT/DOCX附件"
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media/{asset_id}/download [get]
func (h *DocumentHandler) Download(c *gin.Context) {
	project, ok := id(c, "pid")
	if !ok {
		return
	}
	assetID, ok := id(c, "asset_id")
	if !ok {
		return
	}
	actor := identityhttp.Principal(c)
	sources, err := h.sources.Freeze(c.Request.Context(), actor, project, []uuid.UUID{assetID})
	if err != nil {
		writeError(c, err)
		return
	}
	file, err := h.sources.Open(c.Request.Context(), actor, project, sources[0])
	if err != nil {
		if errors.Is(err, application.ErrDocumentSourceConflict) {
			httpapi.WriteProblem(c, http.StatusConflict, "document_source_changed", nil)
		} else {
			writeError(c, err)
		}
		return
	}
	defer func() { _ = file.Close() }()
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": sources[0].FileName})
	if disposition == "" {
		writeError(c, application.ErrUnavailable)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", disposition)
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, file.Size, file.MIMEType, file.File, nil)
}
