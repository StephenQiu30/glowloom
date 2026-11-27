package http

import (
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/subject/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type subjectCursor struct {
	ID      uuid.UUID `json:"id"`
	Time    time.Time `json:"time"`
	Binding string    `json:"binding"`
}

func listInput(query url.Values) (application.ListInput, error) {
	if !httpapi.SingleValueQuery(query, "type", "q", "favorite", "from", "to", "time_field", "order", "page", "page_size", "cursor", "limit") {
		return application.ListInput{}, domain.ErrInvalid
	}
	in := application.ListInput{Type: query.Get("type"), Query: query.Get("q"), TimeField: "updated", Order: "updated_desc", Page: 1, PageSize: 20}
	if query.Has("cursor") || query.Has("limit") {
		if query.Has("page") || query.Has("page_size") {
			return in, domain.ErrInvalid
		}
		in.Page, in.PageSize, in.Limit = 0, 0, 20
		if query.Has("limit") {
			limit, err := strconv.Atoi(query.Get("limit"))
			if err != nil || limit < 1 {
				return in, domain.ErrInvalid
			}
			in.Limit = limit
		}
	}
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

// List returns complete-space filtered results with keyset or offset pagination.
// @Summary 查询当前空间主体
// @ID listSubjects
// @Tags subjects
// @Produce json
// @Param type query string false "主体类型" Enums(character,scene,prop,product,style,other)
// @Param q query string false "名称包含搜索"
// @Param favorite query boolean false "仅收藏"
// @Param from query string false "含开始时间 RFC3339"
// @Param to query string false "不含结束时间 RFC3339"
// @Param time_field query string false "created或updated"
// @Param order query string false "created_asc/created_desc/updated_asc/updated_desc"
// @Param page query int false "页码1..100000"
// @Param page_size query int false "每页1..120"
// @Param limit query int false "游标批大小1..120，默认20，与页码参数互斥"
// @Param cursor query string false "当前查询的上一批游标，与页码参数互斥"
// @Success 200 {object} application.Page
// @Failure 401,403,404,422,503 {object} httpapi.Problem
// @Router /api/subjects [get]
func (h *Handler) List(c *gin.Context) {
	query := c.Request.URL.Query()
	in, err := listInput(query)
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if h == nil || h.query == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	actor := currentActor(c)
	var binding string
	if in.Limit > 0 {
		binding, err = httpapi.CursorBinding(struct {
			Actor, Org, Scope uuid.UUID
			Input             application.ListInput
		}{Actor: actor.ID, Org: actor.OrgID, Scope: actor.ScopeID, Input: in})
		if err != nil {
			writeError(c, err)
			return
		}
		if token := query.Get("cursor"); token != "" {
			var cursor subjectCursor
			if len(token) > 1024 || !httpapi.DecodeCursorToken(token, &cursor) || cursor.Binding != binding || cursor.ID == uuid.Nil || cursor.Time.IsZero() {
				writeError(c, domain.ErrInvalid)
				return
			}
			in.After = &application.ListCursor{ID: cursor.ID, Time: cursor.Time}
		}
	}
	page, err := h.query.List(c.Request.Context(), actor, in)
	if err != nil {
		writeError(c, err)
		return
	}
	if page.Next != nil {
		page.NextCursor, err = httpapi.EncodeCursorToken(subjectCursor{ID: page.Next.ID, Time: page.Next.Time, Binding: binding})
		if err != nil {
			writeError(c, err)
			return
		}
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
