package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// CreateProjectRequest contains only user-editable creation specifications.
type CreateProjectRequest struct {
	Name          string     `json:"name" binding:"required" minLength:"1" maxLength:"50"`
	Description   string     `json:"description"`
	AspectRatio   string     `json:"aspect_ratio" binding:"required" enums:"9:16,16:9"`
	StyleType     string     `json:"style_type" binding:"required" enums:"realistic,stylized"`
	StyleSubtype  string     `json:"style_subtype,omitempty" enums:"anime_jp,guofeng_xianxia,cartoon_3d,manhwa"`
	StylePresetID *uuid.UUID `json:"style_preset_id,omitempty"`
}

// Create atomically registers a project with a zero budget and durable events.
// @Summary 创建项目
// @Tags projects
// @ID createProject
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "24小时内创建请求的 UUID 幂等键"
// @Param body body CreateProjectRequest true "项目规格"
// @Success 201 {object} application.CreatedProject
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects [post]
func (h *Handler) Create(c *gin.Context) {
	var body CreateProjectRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	input := application.CreateProjectInput{Name: body.Name, Description: body.Description, AspectRatio: body.AspectRatio, StyleType: body.StyleType, StyleSubtype: body.StyleSubtype, RequestID: c.GetString("request_id")}
	if body.StylePresetID != nil {
		if *body.StylePresetID == uuid.Nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.StylePresetID = *body.StylePresetID
	}
	created, err := h.create.ExecuteIdempotent(c.Request.Context(), identityhttp.Principal(c), key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(201, created)
}

// StylePresetListResponse is a safe, bounded organization preset page.
type StylePresetListResponse struct {
	Items      []application.StylePresetSummary `json:"items"`
	NextCursor *string                          `json:"next_cursor" extensions:"x-nullable"`
}

type presetCursorToken struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Binding string    `json:"binding"`
}

// A 4096-byte name can require six bytes per character after JSON escaping.
const maxPresetCursorBytes = 32 << 10

// ListPresets reads only existing organization-level presets, without prompts.
// @Summary 查询可用于创建项目的风格预设
// @Tags projects
// @ID listStylePresets
// @Produce json
// @Param style_type query string false "风格类型" Enums(realistic,stylized)
// @Param style_subtype query string false "子风格" Enums(anime_jp,guofeng_xianxia,cartoon_3d,manhwa)
// @Param limit query integer false "每页数量（1～200，未传或0默认50）"
// @Param cursor query string false "与账号、组织及筛选绑定的游标"
// @Success 200 {object} StylePresetListResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/style-presets [get]
func (h *Handler) ListPresets(c *gin.Context) {
	input := application.ListStylePresetsInput{StyleType: c.Query("style_type"), StyleSubtype: c.Query("style_subtype"), Limit: 50}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Limit = limit
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	actor := identityhttp.Principal(c)
	binding := cursorBinding(actor, "style-presets", struct {
		StyleType, StyleSubtype string
		Limit                   int
	}{input.StyleType, input.StyleSubtype, input.Limit})
	if raw := c.Query("cursor"); raw != "" {
		if len(raw) > base64.RawURLEncoding.EncodedLen(maxPresetCursorBytes) {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		var token presetCursorToken
		if err != nil || len(data) > maxPresetCursorBytes || json.Unmarshal(data, &token) != nil || token.ID == uuid.Nil || token.Binding != binding {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = &application.StylePresetCursor{ID: token.ID, Name: token.Name}
	}
	page, err := h.presets.Execute(c.Request.Context(), actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	out := StylePresetListResponse{Items: page.Items}
	if page.Next != nil {
		data, err := json.Marshal(presetCursorToken{page.Next.ID, page.Next.Name, binding})
		if err != nil {
			httpapi.WriteProblem(c, 500, "internal_error", nil)
			return
		}
		token := base64.RawURLEncoding.EncodeToString(data)
		out.NextCursor = &token
	}
	c.JSON(200, out)
}

func cursorBinding(actor identityapp.Principal, resource string, filters any) string {
	// This prevents accidental reuse under another query. Durable authorization
	// is always rechecked independently in PostgreSQL.
	body, _ := json.Marshal(struct {
		Actor, Org uuid.UUID
		Resource   string
		Filters    any
	}{actor.ID, actor.OrgID, resource, filters})
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrProjectNotFound), errors.Is(err, application.ErrStylePresetNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 422, "idempotency_key_reused", nil)
	case errors.Is(err, application.ErrStylePresetMismatch), errors.Is(err, application.ErrInvalidCreateProject), errors.Is(err, application.ErrInvalidStylePresetList):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
