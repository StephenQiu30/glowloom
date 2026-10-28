package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AssetCreationHandler exposes direct new projects and immutable system recipes.
type AssetCreationHandler struct{ creator *application.AssetCreator }

// NewAssetCreationHandler injects the atomic owning-project creation command.
func NewAssetCreationHandler(creator *application.AssetCreator) *AssetCreationHandler {
	return &AssetCreationHandler{creator: creator}
}

// Register installs direct creation and the metadata-only template catalog.
func (h *AssetCreationHandler) Register(group *gin.RouterGroup) {
	group.GET("/canvas-assets/templates", h.Templates)
	group.POST("/canvas-assets", h.Create)
}

// Templates returns explicit system templates rather than invented user assets.
// @Summary 画布与短剧系统模板
// @ID listCanvasAssetTemplates
// @Tags canvases
// @Produce json
// @Param kind query string false "模板分类，省略返回全部" Enums(canvas,short_drama)
// @Success 200 {object} application.AssetTemplateCatalog
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvas-assets/templates [get]
func (h *AssetCreationHandler) Templates(c *gin.Context) {
	items, err := domain.SystemAssetTemplates(c.Query("kind"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, application.AssetTemplateCatalog{Items: items})
}

// Create atomically allocates a new project, main canvas and optional editable recipe.
// @Summary 直接新建画布或短剧
// @ID createAssetCanvas
// @Tags canvases
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body application.AssetCreationInput true "新对象类型与可选系统模板；不执行生成"
// @Success 201 {object} application.CreatedAssetCanvas
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvas-assets [post]
func (h *AssetCreationHandler) Create(c *gin.Context) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	var input application.AssetCreationInput
	if !httpapi.Decode(c, &input) {
		return
	}
	created, err := h.creator.Create(c.Request.Context(), identityhttp.Principal(c), key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(201, created)
}
