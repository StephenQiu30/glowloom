package http

import (
	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// ProviderTrialHandler exposes candidate quotes to platform administrators.
type ProviderTrialHandler struct{ trials application.ProviderTrials }

// NewProviderTrialHandler injects the same transactional quote infrastructure.
func NewProviderTrialHandler(trials application.ProviderTrials) *ProviderTrialHandler {
	return &ProviderTrialHandler{trials: trials}
}

// Register adds quote preparation only; paid execution uses normal confirmation.
func (h *ProviderTrialHandler) Register(group *gin.RouterGroup) {
	group.POST("/admin/model-releases/:id/trial-quotes", h.Quote)
	group.GET("/admin/model-releases/:id/trial-recovery", h.Recovery)
}

// Quote prepares a candidate quote without making a paid provider request.
// @Summary 准备模型试运行报价（不调用供应商）
// @ID createProviderTrialQuote
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "发布ID"
// @Param body body application.ProviderTrialQuote true "模式、项目与预算"
// @Success 201 {object} QuoteResponse
// @Router /api/admin/model-releases/{id}/trial-quotes [post]
func (h *ProviderTrialHandler) Quote(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input application.ProviderTrialQuote
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.trials.CreateProviderTrialQuote(c.Request.Context(), identityhttp.Principal(c), id, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response := quoteResponse(result)
	key := application.ProviderTrialConfirmKey(result.OperationID)
	response.ConfirmKey = &key
	c.JSON(201, response)
}

// Recovery restores a trial without creating a quote or confirming a charge.
// @Summary 恢复原模型试运行
// @ID getProviderTrialRecovery
// @Tags settings
// @Produce json
// @Param id path string true "发布ID"
// @Success 200 {object} application.ProviderTrialRecovery
// @Router /api/admin/model-releases/{id}/trial-recovery [get]
func (h *ProviderTrialHandler) Recovery(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.trials.ReadProviderTrialRecovery(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
