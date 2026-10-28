package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AssetCommandHandler adapts revision-bound personal and lifecycle mutations.
type AssetCommandHandler struct {
	commands *application.AssetCommands
	notify   func(uuid.UUID)
}

// NewAssetCommandHandler injects the object command application.
func NewAssetCommandHandler(commands *application.AssetCommands) *AssetCommandHandler {
	return &AssetCommandHandler{commands: commands}
}

// WithNotification installs a post-commit document refresh hint.
func (h *AssetCommandHandler) WithNotification(notify func(uuid.UUID)) *AssetCommandHandler {
	h.notify = notify
	return h
}

// Register installs the asset mutation collection.
func (h *AssetCommandHandler) Register(group *gin.RouterGroup) {
	group.POST("/canvas-assets/commands", h.Apply)
}

// Apply processes explicit object selections with stable individual outcomes.
// @Summary 画布资产收藏与生命周期操作
// @ID applyCanvasAssetCommand
// @Tags canvases
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body application.AssetCommandInput true "收藏、单项重命名或批量删除；主画布删除回收项目"
// @Success 200 {object} application.AssetCommandReceipt
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvas-assets/commands [post]
func (h *AssetCommandHandler) Apply(c *gin.Context) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	var input application.AssetCommandInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.commands.Apply(c.Request.Context(), identityhttp.Principal(c), key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	if h.notify != nil && input.Action != "favorite" {
		for _, item := range result.Results {
			if item.Status == "succeeded" {
				h.notify(item.ID)
			}
		}
	}
	c.JSON(200, result)
}
