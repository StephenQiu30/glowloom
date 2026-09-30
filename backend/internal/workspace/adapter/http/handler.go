// Package http adapts project queries to the public authenticated API.
package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Handler adapts current-rights project creation and navigation.
type Handler struct {
	list    *application.ListProjectsQuery
	create  *application.CreateProjectCommand
	presets *application.ListStylePresetsQuery
}

// NewHandler injects current-rights project commands and safe navigation queries.
func NewHandler(list *application.ListProjectsQuery, create *application.CreateProjectCommand, presets *application.ListStylePresetsQuery) *Handler {
	return &Handler{list: list, create: create, presets: presets}
}

// ProjectResponse exposes safe project-list fields needed for navigation.
type ProjectResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Revision    int64     `json:"revision"`
	AspectRatio string    `json:"aspect_ratio"`
	StyleType   string    `json:"style_type"`
	IsDelete    bool      `json:"is_delete"`
}

// ListResponse contains one keyset page of visible projects.
type ListResponse struct {
	Items      []ProjectResponse `json:"items"`
	NextCursor *string           `json:"next_cursor" extensions:"x-nullable"`
}
type cursor struct {
	ID      uuid.UUID `json:"id"`
	Time    time.Time `json:"time"`
	Binding string    `json:"binding"`
}

// Register installs authorized project creation and navigation endpoints.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/projects", h.List)
	group.POST("/projects", h.Create)
	group.GET("/style-presets", h.ListPresets)
}

// List returns visible projects and an opaque next cursor.
// @Summary 有权项目列表
// @Tags projects
// @ID listProjects
// @Produce json
// @Param limit query int false "分页大小，上限200"
// @Param cursor query string false "不透明游标"
// @Param q query string false "名称检索"
// @Param status query string false "active或archived"
// @Param deleted query boolean false "只查询回收中的项目"
// @Success 200 {object} ListResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects [get]
func (h *Handler) List(c *gin.Context) {
	input := application.ListProjectsInput{Query: strings.TrimSpace(c.Query("q")), Status: c.Query("status"), Limit: 50}
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Limit = value
	}
	if raw := c.Query("deleted"); raw != "" {
		deleted, err := strconv.ParseBool(raw)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Deleted = deleted
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	actor := identityhttp.Principal(c)
	binding := cursorBinding(actor, "projects", struct {
		Query, Status string
		Deleted       bool
		Limit         int
	}{input.Query, input.Status, input.Deleted, input.Limit})
	if raw := c.Query("cursor"); raw != "" {
		if len(raw) > base64.RawURLEncoding.EncodedLen(512) {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		var cur cursor
		body, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(body) > 512 || json.Unmarshal(body, &cur) != nil || cur.ID == uuid.Nil || cur.Time.IsZero() || cur.Binding != binding {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = &application.ProjectListCursor{ID: cur.ID, UpdateTime: cur.Time}
	}
	page, err := h.list.Execute(c.Request.Context(), actor, input)
	if err != nil {
		switch {
		case errors.Is(err, identityapp.ErrForbidden):
			httpapi.WriteProblem(c, 403, "forbidden", nil)
		case errors.Is(err, application.ErrInvalidProjectList):
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
		default:
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
		}
		return
	}
	response := ListResponse{Items: make([]ProjectResponse, 0, len(page.Projects))}
	for _, p := range page.Projects {
		response.Items = append(response.Items, ProjectResponse{ID: p.ID, Name: p.Name, Status: p.Status, Revision: p.Revision, AspectRatio: p.AspectRatio, StyleType: p.StyleType, IsDelete: p.IsDelete})
	}
	if page.Next != nil {
		body, _ := json.Marshal(cursor{ID: page.Next.ID, Time: page.Next.UpdateTime, Binding: binding})
		value := base64.RawURLEncoding.EncodeToString(body)
		response.NextCursor = &value
	}
	c.JSON(200, response)
}
