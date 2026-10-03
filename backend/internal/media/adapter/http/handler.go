// Package http exposes project-authorized media selection and short-lived previews.
package http

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// Handler is the media query HTTP adapter.
type Handler struct{ query *application.AssetQuery }

// NewHandler injects the authorized media query.
func NewHandler(query *application.AssetQuery) *Handler { return &Handler{query: query} }

// Register installs workspace project-scoped reads.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid/media", h.List)
	group.GET("/projects/:pid/media/:asset_id/preview", h.Preview)
}

// List returns eligible project media.
// @Summary 项目可用媒体列表
// @ID listMediaAssets
// @Tags media
// @Produce json
// @Param pid path string true "项目UUID"
// @Param kind query string false "image/video/audio/model/document；默认仅可渲染媒体"
// @Param cursor query string false "不透明游标"
// @Param limit query integer false "1..200，默认50"
// @Success 200 {object} application.AssetPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media [get]
func (h *Handler) List(c *gin.Context) {
	p, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
	}
	page, err := h.query.List(c.Request.Context(), identityhttp.Principal(c), p, c.Query("kind"), c.Query("cursor"), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, page)
}

// Preview returns a short-lived URL after current media authorization.
// @Summary 获取媒体预览
// @ID getMediaPreview
// @Tags media
// @Produce json
// @Param pid path string true "项目UUID"
// @Param asset_id path string true "媒体UUID"
// @Success 200 {object} application.Preview
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media/{asset_id}/preview [get]
func (h *Handler) Preview(c *gin.Context) {
	p, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	asset, ok := httpapi.PathUUID(c, "asset_id")
	if !ok {
		return
	}
	preview, err := h.query.Preview(c.Request.Context(), identityhttp.Principal(c), p, asset)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, preview)
}
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrInvalidQuery):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
