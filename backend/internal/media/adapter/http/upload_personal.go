package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// UploadPersonal receives one explicitly reviewed file in the current actor's
// personal scope. Multipart, timeouts and admission are shared with projects.
// @Summary 上传人工确认的个人素材
// @ID uploadPersonalMediaAsset
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param Idempotency-Key header string true "UUID永久幂等键"
// @Param file formData file true "与项目媒体相同的真实内容/容量门禁，单文件"
// @Param local_review_confirmed formData boolean true "已确认内容和使用权限且不含需要授权的真人"
// @Success 201 {object} application.PersonalUploadResult
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 413 {object} httpapi.Problem
// @Failure 415 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/uploads [post]
func (h *UploadHandler) UploadPersonal(c *gin.Context) {
	if h == nil || h.service == nil {
		uploadError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	h.receiveUpload(c, uuid.Nil, true)
}
