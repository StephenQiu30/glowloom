package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// PersonalHistoryCommandHandler exposes explicit personal result changes.
type PersonalHistoryCommandHandler struct {
	command *application.PersonalHistoryCommander
}

// NewPersonalHistoryCommandHandler injects the personal result command service.
func NewPersonalHistoryCommandHandler(command *application.PersonalHistoryCommander) *PersonalHistoryCommandHandler {
	return &PersonalHistoryCommandHandler{command: command}
}

// Register installs the personal history mutation endpoint.
func (h *PersonalHistoryCommandHandler) Register(group *gin.RouterGroup) {
	group.POST("/operations/history/commands", h.Apply)
}

// Apply changes only the caller's personal history and returns partial batch outcomes.
// @Summary 收藏或删除本人生成历史
// @ID applyPersonalHistoryCommand
// @Tags operations
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body application.PersonalHistoryCommand true "结果级或整记录个人操作，最大100项"
// @Success 200 {object} application.PersonalHistoryCommandReceipt
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/history/commands [post]
func (h *PersonalHistoryCommandHandler) Apply(c *gin.Context) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var in application.PersonalHistoryCommand
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	receipt, err := h.command.Apply(c.Request.Context(), identityhttp.Principal(c), key, in)
	if errors.Is(err, application.ErrHistoryCommandConflict) {
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
