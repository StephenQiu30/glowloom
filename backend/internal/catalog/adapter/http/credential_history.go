package http

import (
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
)

// CredentialHistoryHandler exposes only safe credential rotation metadata.
type CredentialHistoryHandler struct {
	service *application.CredentialHistory
}

// NewCredentialHistoryHandler injects the history query.
func NewCredentialHistoryHandler(s *application.CredentialHistory) *CredentialHistoryHandler {
	return &CredentialHistoryHandler{service: s}
}

// Register mounts credential history under administrator routes.
func (h *CredentialHistoryHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/providers/:id/credentials", h.List)
}

// List returns active, draining and disabled metadata.
// @Summary 平台 Key 轮换记录
// @ID listProviderCredentials
// @Tags settings
// @Produce json
// @Param id path string true "连接ID"
// @Success 200 {array} application.ProviderCredentialSummary
// @Router /api/admin/providers/{id}/credentials [get]
func (h *CredentialHistoryHandler) List(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, items)
}
