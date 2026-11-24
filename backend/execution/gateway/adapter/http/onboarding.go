package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
)

func bindOnboarding(c *gin.Context, limit int64, out any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}

// model 保存未启用的模型预设与销售价.
// @Summary 保存未启用的模型预设与销售价
// @ID prepareAiModel
// @Tags ai-gateway
// @Accept json
// @Param input body object true "模型预设及销售合同，不含密钥"
// @Success 201
// @Router /api/ai/admin/models [post]
func (h *Handler) model(c *gin.Context) {
	var in domain.Model
	if err := bindOnboarding(c, 64<<10, &in); err != nil {
		problem(c, err)
		return
	}
	if err := h.store(identityhttp.Principal(c)).StageModel(c.Request.Context(), in); err != nil {
		problem(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// route 保存未启用的采购路由和验证计划.
// @Summary 保存采购路由与验证计划
// @ID prepareAiRoute
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param input body object true "路由、采购价、验证付款范围及媒体测试输入，不含密钥"
// @Success 201 {object} pg.OnboardingReview
// @Router /api/ai/admin/routes [post]
func (h *Handler) route(c *gin.Context) {
	var in application.Onboarding
	if err := bindOnboarding(c, 1<<20, &in); err != nil {
		problem(c, err)
		return
	}
	store := h.store(identityhttp.Principal(c))
	if err := store.PrepareOnboarding(c.Request.Context(), in); err != nil {
		problem(c, err)
		return
	}
	writeOnboardingReview(c, store, in.Route.ID, http.StatusCreated)
}

// onboarding 读取候选的验证计划或回执.
// @Summary 读取候选的验证计划或回执
// @ID getAiOnboarding
// @Tags ai-gateway
// @Produce json
// @Param id path string true "候选 ID"
// @Success 200 {object} pg.OnboardingReview
// @Router /api/ai/admin/onboarding/{id} [get]
func (h *Handler) onboarding(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	writeOnboardingReview(c, h.store(identityhttp.Principal(c)), id, http.StatusOK)
}

// verifyOnboarding 提交密钥和预算确认，排队验证并自动启用.
// @Summary 确认预算并提交密钥验证
// @ID verifyAiOnboarding
// @Tags ai-gateway
// @Accept json
// @Produce json
// @Param id path string true "候选 ID"
// @Param input body object true "API Key、确认积分和采购费用上限"
// @Success 202 {object} pg.OnboardingReview
// @Router /api/ai/admin/onboarding/{id}/verify [post]
func (h *Handler) verifyOnboarding(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	var in struct {
		Key            string `json:"key"`
		Credits        *int64 `json:"credits"`
		PurchaseMicros *int64 `json:"purchase_micros"`
	}
	if err != nil || bindOnboarding(c, 16<<10, &in) != nil || in.Credits == nil || in.PurchaseMicros == nil ||
		in.Key == "" || len(in.Key) > 8192 || strings.ContainsAny(in.Key, "\r\n\x00") || strings.TrimSpace(in.Key) != in.Key {
		problem(c, domain.ErrInvalid)
		return
	}
	if h.sealer == nil {
		problem(c, domain.ErrUnavailable)
		return
	}
	store := h.store(identityhttp.Principal(c))
	err = store.ConfirmOnboarding(c.Request.Context(), id, *in.Credits, *in.PurchaseMicros, func(supplier, credential uuid.UUID) ([]byte, string, error) {
		raw, err := json.Marshal(map[string]string{"api_key": in.Key})
		if err != nil {
			return nil, "", err
		}
		defer clear(raw)
		sealed, err := h.sealer.Seal(supplier, credential, raw)
		return sealed, h.sealer.KeyID(), err
	})
	if err != nil {
		problem(c, err)
		return
	}
	writeOnboardingReview(c, store, id, http.StatusAccepted)
}

// rollbackOnboarding 停止候选路线接新单并恢复仍合格的原版本.
// @Summary 回退已启用候选
// @ID rollbackAiOnboarding
// @Tags ai-gateway
// @Param id path string true "候选 ID"
// @Success 204
// @Router /api/ai/admin/onboarding/{id}/rollback [post]
func (h *Handler) rollbackOnboarding(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		problem(c, domain.ErrInvalid)
		return
	}
	if err := h.store(identityhttp.Principal(c)).RollbackOnboarding(c.Request.Context(), id); err != nil {
		problem(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeOnboardingReview(c *gin.Context, store *pg.Store, id uuid.UUID, status int) {
	review, err := store.ReviewOnboarding(c.Request.Context(), id)
	if err != nil {
		problem(c, err)
		return
	}
	c.JSON(status, review)
}

// DeleteRoutesInput identifies the exact channels to retire in one transaction.
type DeleteRoutesInput struct {
	IDs []uuid.UUID `json:"ids" binding:"required"`
}

// @Summary 删除选定的供应商渠道
// @ID deleteAiRoutes
// @Tags ai-gateway
// @Accept json
// @Param input body DeleteRoutesInput true "渠道或待验证候选 ID"
// @Success 204
// @Router /api/ai/admin/routes/delete [post]
func (h *Handler) deleteRoutes(c *gin.Context) {
	var in DeleteRoutesInput
	if err := bindOnboarding(c, 64<<10, &in); err != nil {
		problem(c, err)
		return
	}
	err := h.store(identityhttp.Principal(c)).DeleteRoutes(c.Request.Context(), in.IDs)
	switch {
	case err == nil:
		c.Status(http.StatusNoContent)
	case errors.Is(err, pg.ErrRouteVerifying):
		c.JSON(http.StatusConflict, gin.H{"code": "route_verification_in_progress"})
	case errors.Is(err, pg.ErrRouteNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "route_not_found"})
	default:
		problem(c, err)
	}
}
