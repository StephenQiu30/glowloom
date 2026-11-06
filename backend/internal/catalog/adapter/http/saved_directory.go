package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type savedDirectoryAccepted = application.CredentialTestAccepted

// ReadSavedModelDirectory queues a read-only directory request for the key owner.
// @Summary 使用已保存凭据读取模型目录
// @ID readSavedProviderModelDirectory
// @Tags settings
// @Produce json
// @Param id path string true "连接 ID"
// @Param credential_id path string true "凭据 ID"
// @Param Idempotency-Key header string true "请求 UUID"
// @Success 202 {object} savedDirectoryAccepted
// @Router /api/admin/providers/{id}/credentials/{credential_id}/model-directory [post]
func (h *AdminHandler) ReadSavedModelDirectory(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	credential, ok := httpapi.PathUUID(c, "credential_id")
	if !ok {
		return
	}
	result, err := h.deps.CredentialTest.ExecuteDiscovery(c.Request.Context(), identityhttp.Principal(c), id, credential, c.GetHeader("Idempotency-Key"))
	if err != nil {
		adminError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}
