package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// ProviderLogoHandler exposes normalized supplier image uploads and previews.
type ProviderLogoHandler struct {
	upload *AvatarHandler
	query  *application.ProviderLogoQuery
}

// NewProviderLogoHandler injects the shared image pipeline and authorized query.
func NewProviderLogoHandler(service *application.AvatarService, query *application.ProviderLogoQuery) *ProviderLogoHandler {
	return &ProviderLogoHandler{upload: NewAvatarHandler(service, nil), query: query}
}

// Register requires current session and CSRF middleware on the supplied group.
func (h *ProviderLogoHandler) Register(group *gin.RouterGroup) {
	group.POST("/admin/provider-logos", h.Upload)
	group.GET("/provider-logos/:id", h.Read)
}

// Upload accepts a normalized supplier image without changing any supplier binding.
// @Summary 上传供应商Logo
// @ID uploadProviderLogo
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "PNG/JPEG/WebP image up to 5 MiB"
// @Success 201 {object} application.LibraryAssetSummary
// @Router /api/admin/provider-logos [post]
func (h *ProviderLogoHandler) Upload(c *gin.Context) { h.upload.upload(c) }

// Read returns a short, authorized preview lease.
// @Summary 读取供应商Logo
// @ID getProviderLogo
// @Tags media
// @Produce json
// @Param id path string true "Logo asset UUID"
// @Success 200 {object} application.AvatarPreview
// @Router /api/provider-logos/{id} [get]
func (h *ProviderLogoHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 404, "not_found", nil)
		return
	}
	out, err := h.query.Read(c.Request.Context(), identityhttp.Principal(c), id)
	if errors.Is(err, application.ErrNotFound) {
		httpapi.WriteProblem(c, 404, "not_found", nil)
		return
	}
	if err != nil {
		uploadError(c, err)
		return
	}
	c.JSON(200, out)
}
