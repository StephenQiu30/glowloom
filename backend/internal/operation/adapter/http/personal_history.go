package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
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
	NextCursor *string                           `json:"next_cursor" extensions:"x-nullable"`
}

func personalHistoryInput(values url.Values) (application.PersonalHistoryInput, error) {
	allowed := map[string]bool{"kind": true, "capability": true, "search": true, "created_from": true, "created_before": true, "order": true, "limit": true, "cursor": true, "favorite_only": true, "process_types": true, "resolutions": true, "ratios": true, "generation_modes": true}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return application.PersonalHistoryInput{}, application.ErrInvalidPublicQuery
		}
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
	return input, input.Normalize()
}

func personalHistoryBinding(actor identityapp.Principal, input application.PersonalHistoryInput) (string, error) {
	// After is deliberately absent: a cursor binds the query, not its own position.
	body, err := json.Marshal(struct {
		Actor, Org, Scope uuid.UUID
		Input             application.PersonalHistoryInput
	}{actor.ID, actor.OrgID, actor.ScopeID, input})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
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
// @Param order query string false "desc/asc，默认desc"
// @Param limit query integer false "1..200，默认40"
// @Param cursor query string false "身份、空间与全部条件绑定的游标"
// @Success 200 {object} PersonalHistoryPageResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/history [get]
func (h *PersonalHistoryHandler) List(c *gin.Context) {
	input, err := personalHistoryInput(c.Request.URL.Query())
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	actor := identityhttp.Principal(c)
	binding, err := personalHistoryBinding(actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	if raw := c.Query("cursor"); raw != "" {
		if len(raw) > 1024 {
			writeError(c, application.ErrInvalidPublicQuery)
			return
		}
		var cursor taskCursor
		body, decodeErr := base64.RawURLEncoding.DecodeString(raw)
		if decodeErr != nil || len(body) > 768 || json.Unmarshal(body, &cursor) != nil || cursor.Binding != binding || cursor.ID == uuid.Nil || cursor.CreateTime.IsZero() {
			writeError(c, application.ErrInvalidPublicQuery)
			return
		}
		input.After = &application.TaskCursor{ID: cursor.ID, CreateTime: cursor.CreateTime}
	}
	page, err := h.query.List(c.Request.Context(), actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response := PersonalHistoryPageResponse{Items: page.Items}
	if page.Next != nil {
		body, err := json.Marshal(taskCursor{ID: page.Next.ID, CreateTime: page.Next.CreateTime, Binding: binding})
		if err != nil {
			writeError(c, err)
			return
		}
		encoded := base64.RawURLEncoding.EncodeToString(body)
		response.NextCursor = &encoded
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, response)
}
