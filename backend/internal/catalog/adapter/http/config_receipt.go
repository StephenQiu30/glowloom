package http

import (
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReceiptHandler exposes recoverable safe configuration write receipts.
type ReceiptHandler struct{ service *application.ConfigReceipts }

// NewReceiptHandler injects the receipt query.
func NewReceiptHandler(service *application.ConfigReceipts) *ReceiptHandler {
	return &ReceiptHandler{service: service}
}

// Register mounts actor-scoped receipt recovery.
func (h *ReceiptHandler) Register(group *gin.RouterGroup) {
	group.GET("/admin/config-requests/:id", h.Read)
}

// Read resolves an ambiguous write without resending a secret.
// @Summary 查询配置请求回执
// @ID getConfigReceipt
// @Tags settings
// @Produce json
// @Param id path string true "请求ID"
// @Param action query string true "动作"
// @Param target_id query string true "目标ID，创建为零UUID"
// @Success 200 {object} application.ConfigReceipt
// @Router /api/admin/config-requests/{id} [get]
func (h *ReceiptHandler) Read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	target, err := uuid.Parse(c.Query("target_id"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	result, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), c.Query("action"), target, id)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}
