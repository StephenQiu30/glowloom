package http

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AvatarHandler receives one bounded image for the current account.
type AvatarHandler struct {
	service *application.AvatarService
	query   *application.AvatarQuery
}

// NewAvatarHandler injects the media-owned avatar pipeline.
func NewAvatarHandler(s *application.AvatarService, q *application.AvatarQuery) *AvatarHandler {
	return &AvatarHandler{service: s, query: q}
}

// Register requires current session and CSRF middleware.
func (h *AvatarHandler) Register(group *gin.RouterGroup) {
	group.POST("/me/avatar/uploads", h.upload)
	group.GET("/me/avatar", h.read)
}

// @Summary uploadMyAvatar
// @ID uploadMyAvatar
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "JPEG/PNG/WebP static image up to 5 MiB and 4096x4096"
// @Success 201 {object} application.LibraryAssetSummary
// @Failure 401 {object} httpapi.Problem
// @Failure 413 {object} httpapi.Problem
// @Failure 415 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/me/avatar/uploads [post]
func (h *AvatarHandler) upload(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	actor := identityhttp.Principal(c)
	if err := h.service.Authorize(ctx, actor); err != nil {
		uploadError(c, err)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, application.AvatarMaxBytes+65536)
	typ, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || typ != "multipart/form-data" || params["boundary"] == "" || len(params["boundary"]) > 70 {
		uploadError(c, application.ErrUnsupportedUpload)
		return
	}
	reader := multipart.NewReader(c.Request.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		uploadError(c, application.ErrInvalidUpload)
		return
	}
	defer func() { _ = part.Close() }()
	if part.FormName() != "file" || part.FileName() == "" {
		uploadError(c, application.ErrInvalidUpload)
		return
	}
	data, err := io.ReadAll(io.LimitReader(part, application.AvatarMaxBytes+1))
	if err != nil {
		uploadError(c, err)
		return
	}
	if len(data) > application.AvatarMaxBytes {
		uploadError(c, application.ErrUploadTooLarge)
		return
	}
	if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
		uploadError(c, application.ErrInvalidUpload)
		return
	}
	imageType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil {
		uploadError(c, application.ErrUnsupportedUpload)
		return
	}
	out, err := h.service.Upload(ctx, actor, data, imageType)
	if err != nil {
		uploadError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(201, out)
}

// @Summary getMyAvatar
// @ID getMyAvatar
// @Tags media
// @Produce json
// @Success 200 {object} application.AvatarPreview
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/me/avatar [get]
func (h *AvatarHandler) read(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	out, err := h.query.Read(c.Request.Context(), identityhttp.Principal(c))
	if errors.Is(err, identityapp.ErrAvatarUnavailable) {
		httpapi.WriteProblem(c, 404, "not_found", nil)
		return
	}
	if err != nil {
		uploadError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, out)
}
