package http

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// SourceHandler reuses owning media contracts under a separately checked target space.
type SourceHandler struct {
	sources *media.SubjectLibrary
	uploads SourceUploader
}

// SourceUploader shares the process's existing multipart admission gate.
type SourceUploader interface {
	UploadPersonalFor(*gin.Context, identity.Principal)
}

// NewSourceHandler injects real source queries and the existing bounded uploader.
func NewSourceHandler(sources *media.SubjectLibrary, uploads SourceUploader) *SourceHandler {
	return &SourceHandler{sources: sources, uploads: uploads}
}

// Register exposes no browser-selected source owner or alternate target scope.
func (h *SourceHandler) Register(group *gin.RouterGroup) {
	group.GET("/subjects/sources", h.List)
	group.GET("/subjects/sources/:item_id", h.Detail)
	group.GET("/subjects/sources/:item_id/preview", h.Preview)
	group.POST("/subjects/sources/uploads", h.Upload)
}

func sourceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, media.ErrInvalidQuery), errors.Is(err, mediadomain.ErrInvalidLibrary):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, media.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	default:
		writeError(c, err)
	}
}

// List returns full filtered source pages, authorized for the current target.
// @Summary 选择主体参考资产
// @ID listSubjectSources
// @Tags subjects
// @Produce json
// @Param kind query string true "image/video/audio"
// @Param search query string false "完整素材库搜索"
// @Param favorite_only query bool false "仅收藏"
// @Param quality query string false "upscaled；仅真实超清处理结果"
// @Param page query int false "默认1"
// @Param page_size query int false "默认12；最大120"
// @Success 200 {object} media.SubjectSourcePage
// @Failure 422 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Router /api/subjects/sources [get]
func (h *SourceHandler) List(c *gin.Context) {
	query := c.Request.URL.Query()
	allowed := map[string]bool{"kind": true, "search": true, "favorite_only": true, "quality": true, "page": true, "page_size": true}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			sourceError(c, media.ErrInvalidQuery)
			return
		}
	}
	input := media.SubjectCatalogueQuery{Kind: query.Get("kind"), Search: query.Get("search"), Quality: query.Get("quality"), Page: 1, PageSize: 12}
	for key, destination := range map[string]*int{"page": &input.Page, "page_size": &input.PageSize} {
		if query.Has(key) {
			value, err := strconv.Atoi(query.Get(key))
			if err != nil {
				sourceError(c, media.ErrInvalidQuery)
				return
			}
			*destination = value
		}
	}
	if query.Has("favorite_only") {
		value, err := strconv.ParseBool(query.Get("favorite_only"))
		if err != nil {
			sourceError(c, media.ErrInvalidQuery)
			return
		}
		input.FavoriteOnly = value
	}
	if input.Validate() != nil {
		sourceError(c, media.ErrInvalidQuery)
		return
	}
	result, err := h.sources.List(c.Request.Context(), currentActor(c), input)
	if err != nil {
		sourceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Detail rechecks a selected original before it becomes a formal binding.
// @Summary 核对主体来源资产
// @ID getSubjectSource
// @Tags subjects
// @Produce json
// @Param item_id path string true "资产条目UUID"
// @Success 200 {object} media.SubjectSourceItem
// @Failure 404 {object} httpapi.Problem
// @Router /api/subjects/sources/{item_id} [get]
func (h *SourceHandler) Detail(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		sourceError(c, media.ErrInvalidQuery)
		return
	}
	id, ok := pathID(c, "item_id")
	if !ok {
		return
	}
	result, err := h.sources.Detail(c.Request.Context(), currentActor(c), id)
	if err != nil {
		sourceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Preview returns a private short lease after current source authority.
// @Summary 预览主体来源资产
// @ID previewSubjectSource
// @Tags subjects
// @Produce json
// @Param item_id path string true "资产条目UUID"
// @Success 200 {object} media.LibraryMediaPreview
// @Failure 404 {object} httpapi.Problem
// @Router /api/subjects/sources/{item_id}/preview [get]
func (h *SourceHandler) Preview(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		sourceError(c, media.ErrInvalidQuery)
		return
	}
	id, ok := pathID(c, "item_id")
	if !ok {
		return
	}
	result, err := h.sources.Preview(c.Request.Context(), currentActor(c), id)
	if err != nil {
		sourceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Upload shares the real media admission, byte probing, normalization and replay.
// @Summary 上传主体参考或音色资产
// @ID uploadSubjectSource
// @Tags subjects
// @Accept multipart/form-data
// @Produce json
// @Param Idempotency-Key header string true "UUID永久幂等键"
// @Param file formData file true "单文件；真实媒体上传门禁"
// @Param local_review_confirmed formData boolean true "已确认内容及使用权限"
// @Success 201 {object} media.PersonalUploadResult
// @Failure 422 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Router /api/subjects/sources/uploads [post]
func (h *SourceHandler) Upload(c *gin.Context) {
	actor, err := h.sources.SourceActor(c.Request.Context(), currentActor(c), true)
	if err != nil {
		sourceError(c, err)
		return
	}
	h.uploads.UploadPersonalFor(c, actor)
}
