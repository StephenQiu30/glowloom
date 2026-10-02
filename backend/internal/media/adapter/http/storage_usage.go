package http

import (
	"context"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// StorageUsageQuery exposes a complete authorized inventory, never private keys.
type StorageUsageQuery interface {
	StorageUsage(context.Context, identityapp.Principal, domain.LibraryScope) (application.StorageUsage, error)
}

// StorageUsageHandler serves actual physical occupancy independently of catalog pages.
type StorageUsageHandler struct{ usage StorageUsageQuery }

// NewStorageUsageHandler injects the owning physical query.
func NewStorageUsageHandler(usage StorageUsageQuery) *StorageUsageHandler {
	return &StorageUsageHandler{usage: usage}
}

// Register installs the complete scoped capacity query.
func (h *StorageUsageHandler) Register(group *gin.RouterGroup) {
	group.GET("/media/library/storage-usage", h.Get)
}

// Get returns a bounded physical sample. No hard quota policy is configured.
// @Summary 当前素材库实际私有存储用量
// @ID getMediaLibraryStorageUsage
// @Tags media
// @Produce json
// @Param scope query string false "personal/project；默认personal"
// @Param project_id query string false "project scope必要UUID"
// @Success 200 {object} application.StorageUsage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/storage-usage [get]
func (h *StorageUsageHandler) Get(c *gin.Context) {
	if h == nil || h.usage == nil {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	query := c.Request.URL.Query()
	for key := range query {
		if key != "scope" && key != "project_id" {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
	}
	scope, err := libraryScope(query)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	result, err := h.usage.StorageUsage(c.Request.Context(), identityhttp.Principal(c), scope)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}
