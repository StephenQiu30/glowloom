package http

import (
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

func listInput(query url.Values) (application.ListInput, error) {
	allowed := map[string]bool{"q": true, "favorite": true, "from": true, "to": true, "time_field": true, "order": true, "page": true, "page_size": true}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return application.ListInput{}, domain.ErrInvalid
		}
	}
	in := application.ListInput{Query: query.Get("q"), TimeField: "updated", Order: "updated_desc", Page: 1, PageSize: 40}
	if query.Has("favorite") {
		value := query.Get("favorite")
		if value != "true" && value != "false" {
			return in, domain.ErrInvalid
		}
		in.Favorite = value == "true"
	}
	if query.Has("time_field") {
		in.TimeField = query.Get("time_field")
	}
	if query.Has("order") {
		in.Order = query.Get("order")
	}
	for key, destination := range map[string]*int{"page": &in.Page, "page_size": &in.PageSize} {
		if query.Has(key) {
			value, err := strconv.Atoi(query.Get(key))
			if err != nil {
				return in, domain.ErrInvalid
			}
			*destination = value
		}
	}
	for key, destination := range map[string]**time.Time{"from": &in.From, "to": &in.To} {
		if query.Has(key) {
			value, err := time.Parse(time.RFC3339Nano, query.Get(key))
			if err != nil {
				return in, domain.ErrInvalid
			}
			utc := value.UTC()
			*destination = &utc
		}
	}
	return in, in.Validate()
}

// List returns complete-space filtered results with bounded offset pagination.
// @Summary 查询当前空间主体
// @ID listSubjects
// @Tags subjects
// @Produce json
// @Param q query string false "名称包含搜索"
// @Param favorite query boolean false "仅收藏"
// @Param from query string false "含开始时间 RFC3339"
// @Param to query string false "不含结束时间 RFC3339"
// @Param time_field query string false "created或updated"
// @Param order query string false "created_asc/created_desc/updated_asc/updated_desc"
// @Param page query int false "页码1..100000"
// @Param page_size query int false "每页1..120"
// @Success 200 {object} application.Page
// @Failure 401,403,404,422,503 {object} httpapi.Problem
// @Router /api/subjects [get]
func (h *Handler) List(c *gin.Context) {
	in, err := listInput(c.Request.URL.Query())
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if h == nil || h.query == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	page, err := h.query.List(c.Request.Context(), currentActor(c), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, page)
}

// Get reopens complete current settings without revealing source-library metadata.
// @Summary 读取主体设置
// @ID getSubject
// @Tags subjects
// @Produce json
// @Param id path string true "主体UUID"
// @Success 200 {object} application.Detail
// @Failure 401,403,404,422,503 {object} httpapi.Problem
// @Router /api/subjects/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	if h == nil || h.query == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	detail, err := h.query.Get(c.Request.Context(), currentActor(c), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, detail)
}

// Version exposes safe historical settings while requiring a usable subject head.
// @Summary 读取固定主体版本
// @ID getSubjectVersion
// @Tags subjects
// @Produce json
// @Param id path string true "主体UUID"
// @Param version path string true "不可变版本UUID"
// @Success 200 {object} application.Detail
// @Failure 401,403,404,422,503 {object} httpapi.Problem
// @Router /api/subjects/{id}/versions/{version} [get]
func (h *Handler) Version(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	version, ok := pathID(c, "version")
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	if h == nil || h.query == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	actor := currentActor(c)
	snapshot, err := h.query.ReadVersion(c.Request.Context(), actor, id, version)
	if err != nil {
		writeError(c, err)
		return
	}
	detail, err := h.query.Get(c.Request.Context(), actor, id)
	if err != nil {
		writeError(c, err)
		return
	}
	detail.VersionID = snapshot.ID
	detail.Name = snapshot.Content.Name
	detail.Description = snapshot.Content.Description
	// Public presentation uses current head metadata and pinned version content.
	projected := application.Present(domain.Head{ID: detail.ID, OwnerScopeID: detail.OwnerScopeID, Revision: detail.Revision, CreatedAt: detail.CreatedAt, UpdatedAt: detail.UpdatedAt}, snapshot, detail.Favorite)
	c.JSON(200, projected)
}

// Recover reports a committed command without interpreting absence as failure.
// @Summary 查证主体命令的已提交结果
// @ID getSubjectCommand
// @Tags subjects
// @Produce json
// @Param key path string true "原永久命令UUID"
// @Success 200 {object} application.Recovery
// @Failure 401,403,404,422,503 {object} httpapi.Problem
// @Router /api/subject-commands/{key} [get]
func (h *Handler) Recover(c *gin.Context) {
	key, ok := pathID(c, "key")
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	if h == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	query, ok := h.query.(application.RecoveryQuery)
	if !ok {
		writeError(c, application.ErrUnavailable)
		return
	}
	result, err := query.Recover(c.Request.Context(), currentActor(c), key)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
