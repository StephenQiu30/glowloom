package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// DeleteProviderRequest binds deletion to the inspected connection and drafts.
type DeleteProviderRequest struct {
	Token string `json:"token" binding:"required"`
}

// PreviewProviderDeletion lists affected drafts or explains why deletion is blocked.
// @Summary 预览连接删除
// @ID previewAdminProviderDeletion
// @Tags settings
// @Produce json
// @Param id path string true "连接ID"
// @Success 200 {object} application.ProviderDeletionPreview
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Router /api/admin/providers/{id}/deletion-preview [get]
func (h *AdminHandler) PreviewProviderDeletion(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.deps.ProviderDeletion.Preview(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}

// DeleteProvider removes an unused connection after rechecking the preview.
// @Summary 删除未使用的连接
// @ID deleteAdminProvider
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "连接ID"
// @Param Idempotency-Key header string false "请求标识"
// @Param body body DeleteProviderRequest true "删除预览确认"
// @Success 200 {object} application.DeletedProvider
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/providers/{id} [delete]
func (h *AdminHandler) DeleteProvider(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body DeleteProviderRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	requestID, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		adminError(c, application.ErrInvalidProviderDeletion)
		return
	}
	result, err := h.deps.ProviderDeletion.Delete(c.Request.Context(), identityhttp.Principal(c), id, application.DeleteProviderInput{
		Token: body.Token, RequestID: requestID,
	})
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(200, result)
}
