package http

import (
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
)

// CheckHandler reads progress without revealing supplier errors or credentials.
type CheckHandler struct{ service *application.ProviderChecks }

// NewCheckHandler injects the authorized check reader.
func NewCheckHandler(service *application.ProviderChecks) *CheckHandler {
	return &CheckHandler{service: service}
}

// Register mounts durable check polling.
func (h *CheckHandler) Register(group *gin.RouterGroup) {
	group.GET("/admin/provider-checks/:id", h.Read)
}

// Read reports independent auth, adapter and discovery results.
// @Summary 连接检查与候选模型
// @ID getProviderCheck
// @Tags settings
// @Produce json
// @Param id path string true "检查ID"
// @Success 200 {object} application.ProviderCheck
// @Router /api/admin/provider-checks/{id} [get]
func (h *CheckHandler) Read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}
