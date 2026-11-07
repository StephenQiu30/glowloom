package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// StyleLibraryHandler exposes personal style adoption separately from project presets.
type StyleLibraryHandler struct{ service *app.StyleLibrary }

// NewStyleLibraryHandler injects the style library use cases.
func NewStyleLibraryHandler(service *app.StyleLibrary) *StyleLibraryHandler {
	return &StyleLibraryHandler{service: service}
}

// Register installs the scoped style library endpoints.
func (h *StyleLibraryHandler) Register(g *gin.RouterGroup) {
	g.GET("/style-library", h.List)
	g.GET("/style-library/:id", h.Read)
	g.POST("/style-library", h.Create)
	g.PATCH("/style-library/:id", h.Update)
	g.DELETE("/style-library/:id", h.Delete)
}

// StyleLibraryListResponse is a safe card page; only detail exposes the prompt.
type StyleLibraryListResponse struct {
	Items      []app.StyleLibraryEntry `json:"items"`
	NextCursor *string                 `json:"next_cursor" extensions:"x-nullable"`
}

// StyleLibrarySaveRequest contains the visible style editor fields.
type StyleLibrarySaveRequest struct {
	Name              string      `json:"name"`
	PromptFragment    string      `json:"prompt_fragment"`
	Category          string      `json:"category" enums:"realistic,2d,3d"`
	ReferenceAssetIDs []uuid.UUID `json:"reference_asset_ids"`
}

// StyleLibraryUpdateRequest replaces an observed owned revision.
type StyleLibraryUpdateRequest struct {
	StyleLibrarySaveRequest
	ExpectedRevision *int64 `json:"expected_revision"`
}

// StyleLibraryDeleteRequest confirms the revision to soft-delete.
type StyleLibraryDeleteRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
}

// List returns only current-scope official and/or personally owned cards.
// @Summary 查询风格库
// @Tags styles
// @ID listStyleLibrary
// @Produce json
// @Param q query string false "名称搜索，最多100字"
// @Param category query string false "分类" Enums(realistic,2d,3d)
// @Param mine query boolean false "true仅本人，false仅官方，省略全部可见"
// @Param limit query integer false "默认24，最多100"
// @Param cursor query string false "绑定身份、归属和筛选的游标"
// @Success 200 {object} StyleLibraryListResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/style-library [get]
func (h *StyleLibraryHandler) List(c *gin.Context) {
	in := app.StyleLibraryFilter{Query: c.Query("q"), Category: c.Query("category"), Limit: 24}
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			styleLibraryError(c, app.ErrInvalidStyleLibrary)
			return
		}
		in.Limit = v
	}
	if in.Limit == 0 {
		in.Limit = 24
	}
	if raw := c.Query("mine"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			styleLibraryError(c, app.ErrInvalidStyleLibrary)
			return
		}
		in.Mine = &v
	}
	actor := identityhttp.Principal(c)
	binding := cursorBinding(actor, "style-library", struct {
		Query, Category string
		Mine            *bool
		Limit           int
	}{in.Query, in.Category, in.Mine, in.Limit})
	if raw := c.Query("cursor"); raw != "" {
		var token presetCursorToken
		if len(raw) > base64.RawURLEncoding.EncodedLen(maxPresetCursorBytes) {
			styleLibraryError(c, app.ErrInvalidStyleLibrary)
			return
		}
		b, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(b) > maxPresetCursorBytes || json.Unmarshal(b, &token) != nil || token.Binding != binding || token.ID == uuid.Nil {
			styleLibraryError(c, app.ErrInvalidStyleLibrary)
			return
		}
		in.After = &app.StylePresetCursor{Name: token.Name, ID: token.ID}
	}
	page, err := h.service.List(c.Request.Context(), actor, in)
	if err != nil {
		styleLibraryError(c, err)
		return
	}
	out := StyleLibraryListResponse{Items: page.Items}
	if page.Next != nil {
		b, err := json.Marshal(presetCursorToken{ID: page.Next.ID, Name: page.Next.Name, Binding: binding})
		if err != nil {
			styleLibraryError(c, err)
			return
		}
		v := base64.RawURLEncoding.EncodeToString(b)
		out.NextCursor = &v
	}
	c.JSON(200, out)
}

// Read freezes one currently authorized style for explicit prompt adoption.
// @Summary 读取风格详情
// @Tags styles
// @ID getStyleLibraryItem
// @Produce json
// @Param id path string true "风格ID"
// @Success 200 {object} app.StyleLibraryDetail
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/style-library/{id} [get]
func (h *StyleLibraryHandler) Read(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	out, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		styleLibraryError(c, err)
		return
	}
	c.JSON(200, out)
}

// Create saves the current actor's style without generating images or script.
// @Summary 创建个人风格
// @Tags styles
// @ID createStyleLibraryItem
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body StyleLibrarySaveRequest true "名称≤20字，描述≤2000字，参考图≤4张"
// @Success 201 {object} app.StyleLibraryChangeResult
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/style-library [post]
func (h *StyleLibraryHandler) Create(c *gin.Context) {
	var body StyleLibrarySaveRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	h.change(c, app.StyleLibraryMutation{Action: "create", Name: body.Name, PromptFragment: body.PromptFragment, Category: body.Category, ReferenceAssetIDs: body.ReferenceAssetIDs})
}

// Update replaces only the owning actor's observed style revision.
// @Summary 编辑个人风格
// @Tags styles
// @ID updateStyleLibraryItem
// @Accept json
// @Produce json
// @Param id path string true "风格ID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body StyleLibraryUpdateRequest true "编辑字段及期望版本"
// @Success 200 {object} app.StyleLibraryChangeResult
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/style-library/{id} [patch]
func (h *StyleLibraryHandler) Update(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body StyleLibraryUpdateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		styleLibraryError(c, app.ErrInvalidStyleLibrary)
		return
	}
	h.change(c, app.StyleLibraryMutation{Action: "patch", ID: id, ExpectedRevision: *body.ExpectedRevision, Name: body.Name, PromptFragment: body.PromptFragment, Category: body.Category, ReferenceAssetIDs: body.ReferenceAssetIDs})
}

// Delete hides an unbound owned preset while retaining its immutable image pins.
// @Summary 删除个人风格
// @Tags styles
// @ID deleteStyleLibraryItem
// @Accept json
// @Produce json
// @Param id path string true "风格ID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body StyleLibraryDeleteRequest true "期望版本"
// @Success 200 {object} app.StyleLibraryChangeResult
// @Failure 401 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/style-library/{id} [delete]
func (h *StyleLibraryHandler) Delete(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body StyleLibraryDeleteRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		styleLibraryError(c, app.ErrInvalidStyleLibrary)
		return
	}
	h.change(c, app.StyleLibraryMutation{Action: "delete", ID: id, ExpectedRevision: *body.ExpectedRevision})
}
func (h *StyleLibraryHandler) change(c *gin.Context, in app.StyleLibraryMutation) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		styleLibraryError(c, app.ErrInvalidStyleLibrary)
		return
	}
	in.IdempotencyKey = key
	in.RequestID = c.GetString("request_id")
	out, err := h.service.Change(c.Request.Context(), identityhttp.Principal(c), in)
	if err != nil {
		styleLibraryError(c, err)
		return
	}
	status := 200
	if in.Action == "create" {
		status = 201
	}
	c.JSON(status, out)
}
func styleLibraryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identity.ErrSessionInvalid):
		httpapi.WriteProblem(c, 401, "unauthorized", nil)
	case errors.Is(err, identity.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, app.ErrScopeNotFound), errors.Is(err, app.ErrStylePresetNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, app.ErrInvalidStyleLibrary):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, app.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_key_reused", nil)
	case errors.Is(err, app.ErrStyleLibraryRevision):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, app.ErrStyleLibraryInUse):
		httpapi.WriteProblem(c, 409, "style_preset_in_use", nil)
	case errors.Is(err, app.ErrStyleLibraryReference):
		httpapi.WriteProblem(c, 409, "style_reference_unavailable", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
