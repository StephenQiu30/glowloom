// Package http adapts project queries to the public authenticated API.
package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Handler adapts project-list access without introducing project writes.
type Handler struct {
	list *application.ListProjectsQuery
}

// NewHandler injects the current-rights project query.
func NewHandler(list *application.ListProjectsQuery) *Handler { return &Handler{list: list} }

// ProjectResponse exposes safe project-list fields needed for navigation.
type ProjectResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Revision    int64     `json:"revision"`
	AspectRatio string    `json:"aspect_ratio"`
	StyleType   string    `json:"style_type"`
}

// ListResponse contains one keyset page of visible projects.
type ListResponse struct {
	Items      []ProjectResponse `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}
type cursor struct {
	ID   uuid.UUID `json:"id"`
	Time time.Time `json:"time"`
}

// Register installs the authorized project-list endpoint.
func (h *Handler) Register(group *gin.RouterGroup) { group.GET("/projects", h.List) }

// List returns visible projects and an opaque next cursor.
// @Summary 有权项目列表
// @Tags projects
// @ID listProjects
// @Produce json
// @Param limit query int false "分页大小，上限200"
// @Param cursor query string false "不透明游标"
// @Param q query string false "名称检索"
// @Param status query string false "active或archived"
// @Success 200 {object} ListResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects [get]
func (h *Handler) List(c *gin.Context) {
	input := application.ListProjectsInput{Query: c.Query("q"), Status: c.Query("status")}
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Limit = value
	}
	if raw := c.Query("cursor"); raw != "" {
		var cur cursor
		body, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(body) > 512 || json.Unmarshal(body, &cur) != nil || cur.ID == uuid.Nil || cur.Time.IsZero() {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = &application.ProjectListCursor{ID: cur.ID, UpdateTime: cur.Time}
	}
	page, err := h.list.Execute(c.Request.Context(), identityhttp.Principal(c), input)
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
		response.Items = append(response.Items, ProjectResponse{ID: p.ID, Name: p.Name, Status: p.Status, Revision: p.Revision, AspectRatio: p.AspectRatio, StyleType: p.StyleType})
	}
	if page.Next != nil {
		body, _ := json.Marshal(cursor{ID: page.Next.ID, Time: page.Next.UpdateTime})
		value := base64.RawURLEncoding.EncodeToString(body)
		response.NextCursor = &value
	}
	c.JSON(200, response)
}
