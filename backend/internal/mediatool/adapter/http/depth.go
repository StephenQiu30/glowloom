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

// DepthJobResponse exposes durable facts without private media keys.
type DepthJobResponse = domain.DepthJob

// DepthHandler validates saved source identities for local video depth jobs.
type DepthHandler struct {
	store application.DepthStore
	query *application.DepthQuery
}

// NewDepthHandler injects durable commands and exact subtitle queries.
func NewDepthHandler(store application.DepthStore, query *application.DepthQuery) *DepthHandler {
	return &DepthHandler{store: store, query: query}
}

// Register installs only the explicit local video depth capability.
func (h *DepthHandler) Register(g *gin.RouterGroup) {
	g.POST("/projects/:pid/media-depths", h.Create)
	g.GET("/projects/:pid/media-depths", h.List)
	g.GET("/media-depths/:job_id", h.Get)
	g.GET("/media-depths/:job_id/preview", h.Preview)
	g.GET("/media-depths/:job_id/download", h.Download)
	g.POST("/media-depths/:job_id/review", h.Review)
	g.POST("/media-depths/:job_id/reconcile", h.Reconcile)
	g.POST("/media-depths/:job_id/cancel", h.Cancel)
	g.POST("/media-depths/:job_id/retry", h.Retry)
}
func depthFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrConflict), errors.Is(err, application.ErrCancelled), errors.Is(err, application.ErrDepthCessationUncertain):
		httpapi.WriteProblem(c, 409, "media_depth_conflict", nil)
	case errors.Is(err, application.ErrInvalidDepthInput), errors.Is(err, domain.ErrInvalidDepthJob):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}

// Create freezes a saved audio/video node and accepts durable asynchronous video depth recognition.
// @Summary Create local video depth depth
// @ID createMediaDepth
// @Tags media-depths
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.DepthCreateInput true "Saved source identity"
// @Success 202 {object} DepthJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media-depths [post]
func (h *DepthHandler) Create(c *gin.Context) {
	project, ok := parse(c, c.Param("pid"))
	if !ok {
		return
	}
	var input application.DepthCreateInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := parse(c, c.GetHeader("Idempotency-Key"))
	if !ok {
		return
	}
	job, err := h.store.Create(c.Request.Context(), identityhttp.Principal(c), project, key, input)
	if err != nil {
		depthFail(c, err)
		return
	}
	c.JSON(202, job)
}

// List restores actual depth facts for an optional saved canvas and audio/video node.
// @Summary List project media depths
// @ID listMediaDepths
// @Tags media-depths
// @Produce json
// @Param pid path string true "Project UUID"
// @Param canvas_id query string false "Canvas UUID"
// @Param node_id query string false "Timeline node UUID"
// @Param limit query integer false "1..200, default 50"
// @Param cursor query string false "Project and source bound cursor"
// @Success 200 {object} application.DepthPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media-depths [get]
func (h *DepthHandler) List(c *gin.Context) {
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
		depthFail(c, err)
		return
	}
	page := application.DepthPage{Items: jobs, CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID}
	if len(jobs) > limit {
		page.Items = jobs[:limit]
		raw, err := json.Marshal(cursor{ID: page.Items[limit-1].ID, Binding: binding})
		if err != nil {
			depthFail(c, err)
			return
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		page.NextCursor = &next
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, page)
}

// Get returns current durable facts without private original keys.
// @Summary Get local video depth depth
// @ID getMediaDepth
// @Tags media-depths
// @Produce json
// @Param job_id path string true "Depth UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {object} DepthJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id} [get]
func (h *DepthHandler) Get(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	job, err := h.store.Get(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		depthFail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, job)
}

// Cancel accepts cancellation; cancelled status requires worker cessation evidence.
// @Summary Cancel local video depth depth
// @ID cancelMediaDepth
// @Tags media-depths
// @Accept json
// @Produce json
// @Param job_id path string true "Depth UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Current job revision"
// @Success 202 {object} DepthJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id}/cancel [post]
func (h *DepthHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Retry queues another fenced attempt using immutable source inputs.
// @Summary Retry failed or cancelled local video depth depth
// @ID retryMediaDepth
// @Tags media-depths
// @Accept json
// @Produce json
// @Param job_id path string true "Depth UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Current job revision"
// @Success 202 {object} DepthJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id}/retry [post]
func (h *DepthHandler) Retry(c *gin.Context) { h.control(c, "retry") }
func (h *DepthHandler) control(c *gin.Context, action string) {
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
		depthFail(c, err)
		return
	}
	c.JSON(202, job)
}

// Preview reads or controls only the explicitly authorized depth job.
// @Summary Preview exact local depth output
// @ID previewMediaDepth
// @Tags media-depths
// @Produce json
// @Param job_id path string true "Depth UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {object} application.DepthPreview
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id}/preview [get]
func (h *DepthHandler) Preview(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	preview, err := h.query.Preview(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		depthFail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, preview)
}

// Download reads or controls only the explicitly authorized depth job.
// @Summary Download reviewed local depth output
// @ID downloadMediaDepth
// @Tags media-depths
// @Produce video/mp4
// @Param job_id path string true "Depth UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {file} binary
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id}/download [get]
func (h *DepthHandler) Download(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	reader, size, err := h.query.Download(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		depthFail(c, err)
		return
	}
	defer func() {
		if err := reader.Close(); err != nil {
			_ = c.Error(err)
		}
	}()
	c.Header("Cache-Control", "private, no-store")
	c.DataFromReader(200, size, "video/mp4", io.LimitReader(reader, size), map[string]string{"Content-Disposition": "attachment; filename=depth-" + id.String() + ".mp4"})
}

// Review reads or controls only the explicitly authorized depth job.
// @Summary Review exact local depth output
// @ID reviewMediaDepth
// @Tags media-depths
// @Accept json
// @Produce json
// @Param job_id path string true "Depth UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ReviewInput true "Exact source and review facts"
// @Success 200 {object} DepthJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id}/review [post]
func (h *DepthHandler) Review(c *gin.Context) {
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
		depthFail(c, err)
		return
	}
	c.JSON(200, job)
}

// Reconcile reads or controls only the explicitly authorized depth job.
// @Summary Reconcile original local depth result objects
// @ID reconcileMediaDepth
// @Tags media-depths
// @Accept json
// @Produce json
// @Param job_id path string true "Depth UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Exact source and review facts"
// @Success 202 {object} DepthJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-depths/{job_id}/reconcile [post]
func (h *DepthHandler) Reconcile(c *gin.Context) {
	h.control(c, "reconcile")
}
