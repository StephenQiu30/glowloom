package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
)

type salePreviewInput struct {
	pg.SaleChange
	Usage domain.Usage `json:"usage"`
}

type salePreview struct {
	Credits int64 `json:"credits"`
}

// @Summary 预览模型单次销售积分
// @ID previewAiModelSale
// @Tags ai-gateway
// @Param id path string true "模型"
// @Param input body salePreviewInput true "拟发布售价和样例用量"
// @Success 200 {object} salePreview
// @Router /api/ai/admin/models/{id}/price/preview [post]
func (h *Handler) previewModelSale(c *gin.Context) {
	var in salePreviewInput
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || bindOnboarding(c, 64<<10, &in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	model, err := h.store(identityhttp.Principal(c)).ModelSale(c.Request.Context(), id, in.SaleChange, false)
	if err != nil {
		problem(c, err)
		return
	}
	now := time.Now()
	in.Usage.BillingStartedAt, in.Usage.BillingReceivedAt = now, now
	cost, err := model.Price.Cost(in.Usage)
	if err != nil {
		problem(c, err)
		return
	}
	points, err := domain.Credits(cost)
	if err != nil {
		problem(c, err)
		return
	}
	c.JSON(http.StatusOK, salePreview{Credits: points})
}

// @Summary 发布模型售价
// @ID publishAiModelSale
// @Tags ai-gateway
// @Param id path string true "模型"
// @Param input body pg.SaleChange true "售价与所见版本"
// @Success 200 {object} domain.Model
// @Router /api/ai/admin/models/{id}/price [put]
func (h *Handler) publishModelSale(c *gin.Context) {
	var in pg.SaleChange
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || bindOnboarding(c, 64<<10, &in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	model, err := h.store(identityhttp.Principal(c)).ModelSale(c.Request.Context(), id, in, true)
	if err != nil {
		problem(c, err)
		return
	}
	c.JSON(http.StatusOK, model)
}

// @Summary 发布渠道采购价
// @ID publishAiRoutePurchase
// @Tags ai-gateway
// @Param id path string true "渠道"
// @Param input body pg.SaleChange true "采购价与所见版本"
// @Success 200 {object} domain.Route
// @Router /api/ai/admin/routes/{id}/price [put]
func (h *Handler) publishRoutePurchase(c *gin.Context) {
	var in pg.SaleChange
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || bindOnboarding(c, 64<<10, &in) != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	route, err := h.store(identityhttp.Principal(c)).PublishRoutePurchase(c.Request.Context(), id, in)
	if err != nil {
		problem(c, err)
		return
	}
	c.JSON(http.StatusOK, route)
}
