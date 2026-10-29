package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// ReadDirectorRecovery handles only the authenticated actor recovery draft.
// @Summary 读取导演台私有恢复稿
// @ID readDirectorRecovery
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param nid path string true "导演节点UUID"
// @Success 200 {object} application.DirectorRecoveryResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvases/{id}/director-recovery/{nid} [get]
func (h *Handler) ReadDirectorRecovery(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	node, ok := httpapi.PathUUID(c, "nid")
	if !ok {
		return
	}
	result, err := h.service.ReadDirectorRecovery(c.Request.Context(), identityhttp.Principal(c), id, node)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}

// SaveDirectorRecovery handles only the authenticated actor recovery draft.
// @Summary 保存导演台私有恢复稿
// @ID saveDirectorRecovery
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param nid path string true "导演节点UUID"
// @Param body body application.SaveDirectorRecoveryInput true "恢复稿"
// @Success 200 {object} application.DirectorRecovery
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvases/{id}/director-recovery/{nid} [put]
func (h *Handler) SaveDirectorRecovery(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	node, ok := httpapi.PathUUID(c, "nid")
	if !ok {
		return
	}
	var input application.SaveDirectorRecoveryInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.service.SaveDirectorRecovery(c.Request.Context(), identityhttp.Principal(c), id, node, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}

// DeleteDirectorRecovery handles only the authenticated actor recovery draft.
// @Summary 删除指定版本导演台恢复稿
// @ID deleteDirectorRecovery
// @Tags canvases
// @Accept json
// @Produce json
// @Param id path string true "画布UUID"
// @Param nid path string true "导演节点UUID"
// @Param body body application.DeleteDirectorRecoveryInput true "恢复稿"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/canvases/{id}/director-recovery/{nid} [delete]
func (h *Handler) DeleteDirectorRecovery(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	node, ok := httpapi.PathUUID(c, "nid")
	if !ok {
		return
	}
	var input application.DeleteDirectorRecoveryInput
	if !httpapi.Decode(c, &input) {
		return
	}
	err := h.service.DeleteDirectorRecovery(c.Request.Context(), identityhttp.Principal(c), id, node, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Status(204)
}
