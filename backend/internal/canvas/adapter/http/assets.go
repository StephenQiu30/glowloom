package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// AssetHandler exposes the authorized actual-document inventory.
type AssetHandler struct{ query *application.AssetQuery }

// NewAssetHandler injects the transport-independent asset query.
func NewAssetHandler(query *application.AssetQuery) *AssetHandler {
	return &AssetHandler{query: query}
}

// Register installs the asset collection alongside existing document routes.
func (h *AssetHandler) Register(group *gin.RouterGroup) { group.GET("/canvases", h.List) }

// AssetListResponse contains actual canvases and the opaque next-page position.
type AssetListResponse struct {
	Items      []application.AssetCanvas `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
}

type assetCursor struct {
	ID      uuid.UUID `json:"id"`
	Time    time.Time `json:"time"`
	Binding string    `json:"binding"`
}

// List reads actual canvas or short-drama assets after current scope authorization.
// @Summary 资产画布列表
// @ID listAssetCanvases
// @Tags canvases
// @Produce json
// @Param collection query string false "当前或历史画布来源，默认current" Enums(current,legacy)
// @Param favorite_only query boolean false "仅本人收藏"
// @Param kind query string false "显式画布分类，默认canvas" Enums(canvas,short_drama)
// @Param search query string false "画布名称，最多256字符"
// @Param sort query string false "排序字段，默认updated" Enums(updated,created)
// @Param order query string false "顺序，默认desc" Enums(asc,desc)
// @Param created_from query string false "创建时间下界，RFC3339，包含"
// @Param created_before query string false "创建时间上界，RFC3339，不包含"
// @Param limit query integer false "每页1..200，默认30"
// @Param cursor query string false "绑定身份、空间和全部筛选的游标"
// @Success 200 {object} AssetListResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/canvases [get]
func (h *AssetHandler) List(c *gin.Context) {
	input := application.AssetListInput{Collection: c.Query("collection"), Kind: c.Query("kind"), Search: c.Query("search"), Sort: c.Query("sort"), Order: c.Query("order")}
	invalid := func() { httpapi.WriteProblem(c, 422, "invalid_request", nil) }
	if raw := c.Query("favorite_only"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			invalid()
			return
		}
		input.FavoriteOnly = value
	}
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			invalid()
			return
		}
		input.Limit = value
	}
	for _, field := range []struct {
		name  string
		value **time.Time
	}{{"created_from", &input.CreatedFrom}, {"created_before", &input.CreatedBefore}} {
		if raw := c.Query(field.name); raw != "" {
			value, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil || value.IsZero() {
				invalid()
				return
			}
			value = value.UTC()
			*field.value = &value
		}
	}
	input, err := application.NormalizeAssetListInput(input)
	if err != nil {
		invalid()
		return
	}
	actor := identityhttp.Principal(c)
	body, err := json.Marshal(struct {
		Actor, Org, Scope uuid.UUID
		Input             application.AssetListInput
	}{actor.ID, actor.OrgID, actor.ScopeID, input})
	if err != nil {
		writeError(c, err)
		return
	}
	digest := sha256.Sum256(body)
	binding := hex.EncodeToString(digest[:])
	if raw := c.Query("cursor"); raw != "" {
		if len(raw) > 1024 {
			invalid()
			return
		}
		var cursor assetCursor
		body, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(body) > 768 || json.Unmarshal(body, &cursor) != nil || cursor.ID == uuid.Nil || cursor.Time.IsZero() || cursor.Binding != binding {
			invalid()
			return
		}
		input.After = &application.AssetCursor{ID: cursor.ID, Time: cursor.Time}
	}
	page, err := h.query.List(c.Request.Context(), actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response := AssetListResponse{Items: page.Items}
	if page.Next != nil {
		body, err := json.Marshal(assetCursor{ID: page.Next.ID, Time: page.Next.Time, Binding: binding})
		if err != nil {
			writeError(c, err)
			return
		}
		cursor := base64.RawURLEncoding.EncodeToString(body)
		response.NextCursor = &cursor
	}
	c.JSON(200, response)
}
