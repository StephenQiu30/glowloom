package http

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// PersonalHistoryHandler exposes the authenticated caller's generation history.
type PersonalHistoryHandler struct {
	query *application.PersonalHistoryQuery
}

// NewPersonalHistoryHandler injects the personal history query independently of project tasks.
func NewPersonalHistoryHandler(query *application.PersonalHistoryQuery) *PersonalHistoryHandler {
	return &PersonalHistoryHandler{query: query}
}

// Register installs the static history route alongside existing operation detail routes.
func (h *PersonalHistoryHandler) Register(group *gin.RouterGroup) {
	group.GET("/operations/history", h.List)
}

// PersonalHistoryPageResponse contains summaries and an identity/filter-bound next cursor.
type PersonalHistoryPageResponse struct {
	Items      []application.PersonalHistoryItem `json:"items"`
	DayGroups  []application.PersonalHistoryDay  `json:"day_groups"`
	NextCursor *string                           `json:"next_cursor" extensions:"x-nullable"`
}

func personalHistoryInput(values url.Values) (application.PersonalHistoryInput, error) {
	if !httpapi.SingleValueQuery(values,
		"kind", "capability", "search", "order", "limit", "cursor", "favorite_only",
		"created_from", "created_before", "time_zone", "from_day", "through_day",
		"process_types", "resolutions", "ratios", "generation_modes",
	) {
		return application.PersonalHistoryInput{}, application.ErrInvalidPublicQuery
	}
	input := application.PersonalHistoryInput{Kind: values.Get("kind"), Capability: values.Get("capability"), Search: values.Get("search"), Order: values.Get("order")}
	for key, dst := range map[string]*[]string{"process_types": &input.ProcessTypes, "resolutions": &input.Resolutions, "ratios": &input.Ratios, "generation_modes": &input.GenerationModes} {
		if values.Has(key) {
			*dst = strings.Split(values.Get(key), ",")
		}
	}
	if values.Has("favorite_only") {
		if values.Get("favorite_only") != "true" && values.Get("favorite_only") != "false" {
			return input, application.ErrInvalidPublicQuery
		}
		input.FavoriteOnly = values.Get("favorite_only") == "true"
	}
	if values.Has("limit") {
		n, err := strconv.Atoi(values.Get("limit"))
		if err != nil || n < 1 {
			return input, application.ErrInvalidPublicQuery
		}
		input.Limit = n
	}
	for key, destination := range map[string]**time.Time{"created_from": &input.CreatedFrom, "created_before": &input.CreatedBefore} {
		if values.Has(key) {
			t, err := time.Parse(time.RFC3339Nano, values.Get(key))
			if err != nil {
				return input, application.ErrInvalidPublicQuery
			}
			*destination = &t
		}
	}
	return input, nil
}

// List returns only proven personally initiated tasks with current project access.
// @Summary 本人发起的生成历史
// @ID listPersonalOperationHistory
// @Tags operations
// @Produce json
// @Param kind query string false "image/video/audio/document；按能力族分类，含处理中与失败"
// @Param capability query string false "生成能力"
// @Param search query string false "模型名或冻结输入文本，最长200字符"
// @Param favorite_only query boolean false "仅收藏结果"
// @Param process_types query string false "super_resolution,frame_interpolation，逗号分隔"
// @Param resolutions query string false "1K,2K,4K,8K,720P,1080P，逗号分隔"
// @Param ratios query string false "画面比例（如16:9），逗号分隔"
// @Param generation_modes query string false "all_reference,first_last_frame,multi_frame,lip_sync,motion_mimic,song,instrumental,tts，逗号分隔"
// @Param created_from query string false "RFC3339创建时间起点，包含"
// @Param created_before query string false "RFC3339创建时间终点，不包含"
// @Param time_zone query string false "IANA 时区，默认 UTC，决定日历日期和日期分组"
// @Param from_day query string false "创建日期起点 YYYY-MM-DD，包含当天"
// @Param through_day query string false "创建日期终点 YYYY-MM-DD，包含当天"
// @Param order query string false "desc/asc，默认desc"
// @Param limit query integer false "1..200，默认20"
// @Param cursor query string false "身份、空间与全部条件绑定的游标"
// @Success 200 {object} PersonalHistoryPageResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/history [get]
func (h *PersonalHistoryHandler) List(c *gin.Context) {
	values := c.Request.URL.Query()
	input, err := personalHistoryInput(values)
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	zone := values.Get("time_zone")
	location, err := application.ApplyPersonalHistoryCalendar(&input, zone, values.Get("from_day"), values.Get("through_day"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	actor := identityhttp.Principal(c)
	pagination, err := newTaskPagination(struct {
		Actor, Org, Scope uuid.UUID
		Input             application.PersonalHistoryInput
		TimeZone          string `json:",omitempty"`
	}{Actor: actor.ID, Org: actor.OrgID, Scope: actor.ScopeID, Input: input, TimeZone: zone})
	if err != nil {
		writeError(c, err)
		return
	}
	input.After, err = pagination.after(values.Get("cursor"))
	if err != nil {
		writeError(c, err)
		return
	}
	page, err := h.query.List(c.Request.Context(), actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	nextCursor, err := pagination.next(page.Next)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, PersonalHistoryPageResponse{
		Items:      page.Items,
		DayGroups:  application.GroupPersonalHistoryDays(page.Items, location, time.Now()),
		NextCursor: nextCursor,
	})
}
