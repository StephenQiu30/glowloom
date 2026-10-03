package http

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// LibraryHandler exposes only the current caller's closed personal/project scope.
type LibraryHandler struct{ library application.LibraryRepository }

// NewLibraryHandler injects the owning catalog commands and full-page reads.
func NewLibraryHandler(library application.LibraryRepository) *LibraryHandler {
	return &LibraryHandler{library: library}
}

// Register installs metadata reads and permanent commands; file handling stays
// with the existing upload/private-media owner rather than metadata endpoints.
func (h *LibraryHandler) Register(group *gin.RouterGroup) {
	group.GET("/media/library", h.List)
	group.GET("/media/library/items/:item_id", h.Detail)
	group.POST("/media/library/commands", h.Command)
}

func libraryScope(query url.Values) (domain.LibraryScope, error) {
	allowed := map[string]bool{"scope": true, "project_id": true, "page": true, "page_size": true, "kind": true, "category": true, "folder_id": true, "root_only": true, "favorite_only": true, "recent_only": true, "catalog_state": true, "search": true, "order": true}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return domain.LibraryScope{}, domain.ErrInvalidLibrary
		}
	}
	scope := domain.LibraryScope{Kind: domain.LibraryKind(query.Get("scope"))}
	if scope.Kind == "" {
		scope.Kind = domain.LibraryPersonal
	}
	if query.Has("project_id") {
		id, err := uuid.Parse(query.Get("project_id"))
		if err != nil || id == uuid.Nil {
			return domain.LibraryScope{}, domain.ErrInvalidLibrary
		}
		scope.ProjectID = &id
	}
	return scope, scope.Validate()
}

func libraryPageQuery(query url.Values, scope domain.LibraryScope) (application.LibraryQuery, error) {
	page := application.LibraryQuery{Page: 1, PageSize: 40, Kind: query.Get("kind"), Category: query.Get("category"), State: "active", Search: query.Get("search"), Order: "updated_desc"}
	if scope.Kind == domain.LibraryProject {
		page.PageSize = 20
	}
	for key, destination := range map[string]*int{"page": &page.Page, "page_size": &page.PageSize} {
		if query.Has(key) {
			value, err := strconv.Atoi(query.Get(key))
			if err != nil {
				return page, domain.ErrInvalidLibrary
			}
			*destination = value
		}
	}
	for key, destination := range map[string]*bool{"root_only": &page.RootOnly, "favorite_only": &page.FavoriteOnly, "recent_only": &page.RecentOnly} {
		if query.Has(key) {
			value, err := strconv.ParseBool(query.Get(key))
			if err != nil {
				return page, domain.ErrInvalidLibrary
			}
			*destination = value
		}
	}
	if query.Has("folder_id") {
		value, err := uuid.Parse(query.Get("folder_id"))
		if err != nil || value == uuid.Nil {
			return page, domain.ErrInvalidLibrary
		}
		page.FolderID = &value
	}
	if query.Has("catalog_state") {
		page.State = query.Get("catalog_state")
	}
	if query.Has("order") {
		page.Order = query.Get("order")
	}
	return page, page.Validate(scope)
}

// List returns fully filtered and sorted server-side pages, never loaded-only filtering.
// @Summary 个人或项目完整素材库分页
// @ID listMediaLibrary
// @Tags media
// @Produce json
// @Param scope query string false "personal/project；默认personal"
// @Param project_id query string false "project scope必要UUID"
// @Param page query int false "页码1..100000；默认1"
// @Param page_size query int false "每页1..120；个人默认40/项目默认20"
// @Param kind query string false "text/image/video/audio/document/model"
// @Param category query string false "character/environment/prop/material/other"
// @Param folder_id query string false "当前库目录UUID"
// @Param root_only query bool false "只看未分类根目录"
// @Param favorite_only query bool false "个人收藏"
// @Param recent_only query bool false "最近30日"
// @Param catalog_state query string false "active/trashed；默认active"
// @Param search query string false "完整库文字搜索"
// @Param order query string false "updated_desc/updated_asc/name_asc"
// @Success 200 {object} application.LibraryPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library [get]
func (h *LibraryHandler) List(c *gin.Context) {
	if h == nil || h.library == nil {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	scope, err := libraryScope(c.Request.URL.Query())
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	query, err := libraryPageQuery(c.Request.URL.Query(), scope)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	page, err := h.library.ListLibrary(c.Request.Context(), identityhttp.Principal(c), scope, query)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, page)
}

// Detail returns typed editable content; no private file URL or object key is exposed.
// @Summary 当前素材库条目详情
// @ID getMediaLibraryItem
// @Tags media
// @Produce json
// @Param item_id path string true "条目UUID"
// @Param scope query string false "personal/project"
// @Param project_id query string false "project scope必要UUID"
// @Success 200 {object} application.LibraryItemDetail
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/items/{item_id} [get]
func (h *LibraryHandler) Detail(c *gin.Context) {
	if h == nil || h.library == nil {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	scope, err := libraryScope(c.Request.URL.Query())
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	for key := range c.Request.URL.Query() {
		if key != "scope" && key != "project_id" {
			writeLibraryError(c, domain.ErrInvalidLibrary)
			return
		}
	}
	item, ok := httpapi.PathUUID(c, "item_id")
	if !ok {
		return
	}
	detail, err := h.library.LibraryDetail(c.Request.Context(), identityhttp.Principal(c), scope, item)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, detail)
}

// Command applies one atomic metadata action with permanent actor/key replay.
// @Summary 修改素材库目录或元数据
// @ID applyMediaLibraryCommand
// @Tags media
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID永久幂等键"
// @Param body body application.LibraryCommand true "闭合scope、revision及编辑动作"
// @Success 200 {object} application.LibraryReceipt
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/commands [post]
func (h *LibraryHandler) Command(c *gin.Context) {
	if h == nil || h.library == nil {
		writeLibraryError(c, application.ErrUnavailable)
		return
	}
	var command application.LibraryCommand
	if !httpapi.Decode(c, &command) {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		writeLibraryError(c, domain.ErrInvalidLibrary)
		return
	}
	command.Key = key
	result, err := h.library.ApplyLibraryCommand(c.Request.Context(), identityhttp.Principal(c), command)
	if err != nil {
		writeLibraryError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

func writeLibraryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrLibraryKeyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrLibraryConflict):
		httpapi.WriteProblem(c, 409, "library_revision_conflict", nil)
	case errors.Is(err, application.ErrLibraryFolderNotEmpty):
		httpapi.WriteProblem(c, 409, "library_folder_not_empty", nil)
	case errors.Is(err, domain.ErrMediaStateConflict):
		httpapi.WriteProblem(c, 409, "media_state_conflict", nil)
	case errors.Is(err, domain.ErrInvalidLibrary):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
