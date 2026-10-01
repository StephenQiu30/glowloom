// Package http adapts authorized local exports without accepting arbitrary URLs.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// ExportJobResponse names the safe application result for the generated client.
type ExportJobResponse = domain.ExportJob

// Handler owns HTTP validation and application command/query dependencies.
type Handler struct {
	store application.ExportStore
	query *application.ExportQuery
}

// NewHandler injects the formal job commands and private result queries.
func NewHandler(store application.ExportStore, query *application.ExportQuery) *Handler {
	return &Handler{store: store, query: query}
}

// Register installs project creation/list and scoped local job commands.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/projects/:pid/media-exports", h.Create)
	g.GET("/projects/:pid/media-exports", h.List)
	g.GET("/media-exports/:job_id", h.Get)
	g.GET("/media-exports/:job_id/preview", h.Preview)
	g.POST("/media-exports/:job_id/review", h.Review)
	g.POST("/media-exports/:job_id/cancel", h.Cancel)
	g.POST("/media-exports/:job_id/retry", h.Retry)
	g.GET("/media-exports/:job_id/download", h.Download)
	g.GET("/media-exports/:job_id/subtitles", h.Subtitles)
}
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrConflict) || errors.Is(err, application.ErrCancelled):
		httpapi.WriteProblem(c, 409, "media_export_conflict", nil)
	case errors.Is(err, application.ErrInvalidExport):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
func parse(c *gin.Context, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return uuid.Nil, false
	}
	return id, true
}
func jobScope(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	id, ok := parse(c, c.Param("job_id"))
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	project, ok := parse(c, c.Query("project_id"))
	return project, id, ok
}

// Create freezes a saved timeline and accepts durable asynchronous rendering.
// @Summary Create local timeline export
// @ID createMediaExport
// @Tags media-exports
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.CreateInput true "Saved source identity"
// @Success 202 {object} ExportJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media-exports [post]
func (h *Handler) Create(c *gin.Context) {
	project, ok := parse(c, c.Param("pid"))
	if !ok {
		return
	}
	var input application.CreateInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := parse(c, c.GetHeader("Idempotency-Key"))
	if !ok {
		return
	}
	job, err := h.store.Create(c.Request.Context(), identityhttp.Principal(c), project, key, input)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(202, job)
}

type cursor struct {
	ID      uuid.UUID `json:"id"`
	Binding string    `json:"binding"`
}

// List restores actual export facts for an optional saved canvas and timeline node.
// @Summary List project media exports
// @ID listMediaExports
// @Tags media-exports
// @Produce json
// @Param pid path string true "Project UUID"
// @Param canvas_id query string false "Canvas UUID"
// @Param node_id query string false "Timeline node UUID"
// @Param limit query integer false "1..200, default 50"
// @Param cursor query string false "Project and source bound cursor"
// @Success 200 {object} application.ExportPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media-exports [get]
func (h *Handler) List(c *gin.Context) {
	project, ok := parse(c, c.Param("pid"))
	if !ok {
		return
	}
	input := application.ListInput{ProjectID: project, Limit: 50}
	for _, f := range []struct {
		name string
		dest *uuid.UUID
	}{{"canvas_id", &input.CanvasID}, {"node_id", &input.NodeID}} {
		if raw := c.Query(f.name); raw != "" {
			*f.dest, ok = parse(c, raw)
			if !ok {
				return
			}
		}
	}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Limit = n
	}
	actor := identityhttp.Principal(c)
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s/%s/%s/%d", actor.ID, actor.OrgID, project, input.CanvasID, input.NodeID, input.Limit)))
	binding := hex.EncodeToString(hash[:])
	if raw := c.Query("cursor"); raw != "" {
		var cur cursor
		body, err := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 768 || err != nil || json.Unmarshal(body, &cur) != nil || cur.ID == uuid.Nil || cur.Binding != binding {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = cur.ID
	}
	limit := input.Limit
	input.Limit++
	jobs, err := h.store.List(c.Request.Context(), actor, input)
	if err != nil {
		fail(c, err)
		return
	}
	page := application.ExportPage{Items: jobs}
	if len(jobs) > limit {
		page.Items = jobs[:limit]
		raw, err := json.Marshal(cursor{ID: page.Items[limit-1].ID, Binding: binding})
		if err != nil {
			fail(c, err)
			return
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		page.NextCursor = &next
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, page)
}

// Get returns current durable facts without private original keys.
// @Summary Get local media export
// @ID getMediaExport
// @Tags media-exports
// @Produce json
// @Param job_id path string true "Export UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {object} ExportJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id} [get]
func (h *Handler) Get(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	job, err := h.store.Get(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, job)
}

// Preview signs the actual produced bytes for explicit post-render inspection.
// @Summary Preview pending or reviewed media export
// @ID previewMediaExport
// @Tags media-exports
// @Produce json
// @Param job_id path string true "Export UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {object} application.ExportPreview
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id}/preview [get]
func (h *Handler) Preview(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	preview, err := h.query.Preview(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, preview)
}

// Review records the owner's rights and nonrestricted-real-person declaration
// only after inspecting the exact produced SHA at the current job revision.
// @Summary Confirm review of actual exported file
// @ID reviewMediaExport
// @Tags media-exports
// @Accept json
// @Produce json
// @Param job_id path string true "Export UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ReviewInput true "SHA, revision and explicit owner review"
// @Success 200 {object} ExportJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id}/review [post]
func (h *Handler) Review(c *gin.Context) {
	id, ok := parse(c, c.Param("job_id"))
	if !ok {
		return
	}
	var input application.ReviewInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := parse(c, c.GetHeader("Idempotency-Key"))
	if !ok {
		return
	}
	job, err := h.store.Review(c.Request.Context(), identityhttp.Principal(c), id, key, input)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, job)
}

// Cancel accepts cancellation; cancelled status requires worker cessation evidence.
// @Summary Cancel local media export
// @ID cancelMediaExport
// @Tags media-exports
// @Accept json
// @Produce json
// @Param job_id path string true "Export UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Current job revision"
// @Success 202 {object} ExportJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Retry queues another fenced attempt using immutable source inputs.
// @Summary Retry failed or cancelled local media export
// @ID retryMediaExport
// @Tags media-exports
// @Accept json
// @Produce json
// @Param job_id path string true "Export UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Current job revision"
// @Success 202 {object} ExportJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id}/retry [post]
func (h *Handler) Retry(c *gin.Context) { h.control(c, "retry") }
func (h *Handler) control(c *gin.Context, action string) {
	id, ok := parse(c, c.Param("job_id"))
	if !ok {
		return
	}
	var input application.ControlInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := parse(c, c.GetHeader("Idempotency-Key"))
	if !ok {
		return
	}
	job, err := h.store.Control(c.Request.Context(), identityhttp.Principal(c), input.ProjectID, id, key, input.Revision, action)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(202, job)
}

// Download streams one reviewed MP4 or M4A as a private attachment.
// @Summary Download reviewed media export
// @ID downloadMediaExport
// @Tags media-exports
// @Produce video/mp4,audio/mp4
// @Param job_id path string true "Export UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {file} binary
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id}/download [get]
func (h *Handler) Download(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	reader, size, err := h.query.Download(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		fail(c, err)
		return
	}
	defer func() { _ = reader.Close() }()
	c.Header("Cache-Control", "private, no-store")
	job, err := h.store.Get(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		fail(c, err)
		return
	}
	mime, ext := "video/mp4", "mp4"
	if job.OutputKind == domain.OutputAudio {
		mime, ext = "audio/mp4", "m4a"
	}
	c.DataFromReader(200, size, mime, io.LimitReader(reader, size), map[string]string{"Content-Disposition": "attachment; filename=timeline-" + id.String() + "." + ext})
}

// Subtitles downloads SRT from visible frozen captions without a transcription claim.
// @Summary Download timeline subtitles as SRT
// @ID downloadMediaExportSubtitles
// @Tags media-exports
// @Produce application/x-subrip
// @Param job_id path string true "Export UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {file} binary
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-exports/{job_id}/subtitles [get]
func (h *Handler) Subtitles(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	text, err := h.query.Subtitles(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", "attachment; filename=timeline-"+id.String()+".srt")
	c.Data(200, "application/x-subrip; charset=utf-8", []byte(text))
}
