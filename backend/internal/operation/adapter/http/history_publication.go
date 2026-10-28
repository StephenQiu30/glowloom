package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// HistoryPublicationHandler separates authenticated publishing from public projections.
type HistoryPublicationHandler struct {
	command *application.HistoryPublisher
	query   *application.PublishedWorkQuery
}

// NewHistoryPublicationHandler injects authenticated writes and public reads.
func NewHistoryPublicationHandler(command *application.HistoryPublisher, query *application.PublishedWorkQuery) *HistoryPublicationHandler {
	return &HistoryPublicationHandler{command: command, query: query}
}

// Register keeps publication writes behind the current session middleware.
func (h *HistoryPublicationHandler) Register(protected, public *gin.RouterGroup) {
	protected.POST("/operations/history/publications", h.Publish)
	public.GET("/works/:work_id", h.Get)
}

// Publish publishes only the explicitly selected image/video candidates.
// @Summary 发布本人生成结果
// @ID publishHistoryResults
// @Tags operations
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body application.HistoryPublicationInput true "明确选择公开的作品与提示词"
// @Success 200 {object} application.HistoryPublicationReceipt
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/history/publications [post]
func (h *HistoryPublicationHandler) Publish(c *gin.Context) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var in application.HistoryPublicationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	receipt, err := h.command.Publish(c.Request.Context(), identityhttp.Principal(c), key, in)
	if errors.Is(err, application.ErrPublicationConflict) {
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, receipt)
}

// Get returns only fields explicitly chosen for a live public work.
// @Summary 查看已发布作品
// @ID getPublishedWork
// @Tags works
// @Produce json
// @Param work_id path string true "作品UUID"
// @Success 200 {object} application.PublishedWork
// @Failure 404 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/works/{work_id} [get]
func (h *HistoryPublicationHandler) Get(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "work_id")
	if !ok {
		return
	}
	work, err := h.query.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex")
	c.JSON(http.StatusOK, work)
}
