package http

import (
	"strconv"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// Information returns current canvas metadata and its settled accounting facts.
// @Summary 画布项目信息与费用明细
// @ID getCanvasInformation
// @Tags canvases
// @Produce json
// @Param id path string true "画布UUID"
// @Param page query integer false "明细页，从1开始，每页20条" minimum(1) maximum(10000)
// @Success 200 {object} application.CanvasInformation
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases/{id}/information [get]
func (h *Handler) Information(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	var result application.CanvasInformation
	result, err = h.service.Information(c.Request.Context(), identityhttp.Principal(c), id, page)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
