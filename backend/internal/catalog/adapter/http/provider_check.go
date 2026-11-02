package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
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
	group.GET("/admin/providers/:id/checks", h.List)
	group.GET("/admin/providers/:id/test-models", h.Models)
	group.POST("/admin/providers/:id/checks/reachability", h.Network)
	group.POST("/admin/provider-checks/:id/query", h.Query)
	group.POST("/admin/provider-checks/:id/end-tracking", h.EndTracking)
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

// List returns a page of current and historical check evidence.
// @Summary 服务测试记录
// @ID listProviderChecks
// @Tags settings
// @Produce json
// @Param id path string true "服务ID"
// @Param offset query int false "记录偏移"
// @Success 200 {object} application.ProviderCheckPage
// @Router /api/admin/providers/{id}/checks [get]
func (h *CheckHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		presetError(c, application.ErrInvalidCredentialTest)
		return
	}
	result, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), id, offset)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}

// Network schedules an explicit no-key probe at the provider worker.
// @Summary 检测服务地址连通
// @ID testProviderReachability
// @Tags settings
// @Produce json
// @Param id path string true "服务ID"
// @Param Idempotency-Key header string true "请求UUID"
// @Success 202 {object} application.CredentialTestAccepted
// @Router /api/admin/providers/{id}/checks/reachability [post]
func (h *CheckHandler) Network(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.Network(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"))
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(202, result)
}

// Query schedules a read of the original upstream task, never another generation.
// @Summary 查询原模型验证任务
// @ID queryProviderModelCheck
// @Tags settings
// @Produce json
// @Param id path string true "检查ID"
// @Param Idempotency-Key header string true "请求UUID"
// @Success 202 {object} application.CredentialTestAccepted
// @Router /api/admin/provider-checks/{id}/query [post]
func (h *CheckHandler) Query(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.Query(c.Request.Context(), identityhttp.Principal(c), id, c.GetHeader("Idempotency-Key"))
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(202, result)
}

// EndTrackingInput records a nonsecret manual investigation without changing evidence.
type EndTrackingInput struct {
	RequestID uuid.UUID `json:"request_id"`
	Note      string    `json:"note"`
}

// EndTracking preserves unknown evidence and all original accounting state.
// @Summary 结束追踪并保留未知
// @ID endProviderCheckTracking
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "检查ID"
// @Param body body EndTrackingInput true "核查说明"
// @Success 200 {object} application.ProviderCheck
// @Router /api/admin/provider-checks/{id}/end-tracking [post]
func (h *CheckHandler) EndTracking(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var input EndTrackingInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.EndTracking(c.Request.Context(), identityhttp.Principal(c), id, input.RequestID, input.Note)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}

// Models returns the saved validation targets of one connection.
// @Summary 服务可测试模型
// @ID listProviderTestModels
// @Tags settings
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {array} application.ProviderTestModel
// @Router /api/admin/providers/{id}/test-models [get]
func (h *CheckHandler) Models(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.Models(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		presetError(c, err)
		return
	}
	c.JSON(200, result)
}
