package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// TranscriptionJobResponse exposes durable facts without private media keys.
type TranscriptionJobResponse = domain.TranscriptionJob

// TranscriptionHandler validates saved source identities for local speech jobs.
type TranscriptionHandler struct {
	store application.TranscriptionStore
}

// NewTranscriptionHandler injects durable commands and exact subtitle queries.
func NewTranscriptionHandler(store application.TranscriptionStore) *TranscriptionHandler {
	return &TranscriptionHandler{store: store}
}

// Register installs only the explicit local speech capability.
func (h *TranscriptionHandler) Register(g *gin.RouterGroup) {
	g.POST("/projects/:pid/media-transcriptions", h.Create)
	g.GET("/projects/:pid/media-transcriptions", h.List)
	g.GET("/media-transcriptions/:job_id", h.Get)
	g.GET("/media-transcriptions/:job_id/result", h.Result)
	g.GET("/media-transcriptions/:job_id/subtitles", h.Subtitles)
	g.POST("/media-transcriptions/:job_id/cancel", h.Cancel)
	g.POST("/media-transcriptions/:job_id/retry", h.Retry)
}
func transcriptionFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrConflict), errors.Is(err, application.ErrCancelled), errors.Is(err, application.ErrInferenceUncertain):
		httpapi.WriteProblem(c, 409, "media_transcription_conflict", nil)
	case errors.Is(err, application.ErrInvalidTranscription), errors.Is(err, domain.ErrInvalidTranscript):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}

// Create freezes a saved audio/video node and accepts durable asynchronous speech recognition.
// @Summary Create local speech transcription
// @ID createMediaTranscription
// @Tags media-transcriptions
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.TranscriptionCreateInput true "Saved source identity"
// @Success 202 {object} TranscriptionJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media-transcriptions [post]
func (h *TranscriptionHandler) Create(c *gin.Context) {
	project, ok := parse(c, c.Param("pid"))
	if !ok {
		return
	}
	var input application.TranscriptionCreateInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := parse(c, c.GetHeader("Idempotency-Key"))
	if !ok {
		return
	}
	job, err := h.store.Create(c.Request.Context(), identityhttp.Principal(c), project, key, input)
	if err != nil {
		transcriptionFail(c, err)
		return
	}
	c.JSON(202, job)
}

// List restores actual transcription facts for an optional saved canvas and audio/video node.
// @Summary List project media transcriptions
// @ID listMediaTranscriptions
// @Tags media-transcriptions
// @Produce json
// @Param pid path string true "Project UUID"
// @Param canvas_id query string false "Canvas UUID"
// @Param node_id query string false "Timeline node UUID"
// @Param limit query integer false "1..200, default 50"
// @Param cursor query string false "Project and source bound cursor"
// @Success 200 {object} application.TranscriptionPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/media-transcriptions [get]
func (h *TranscriptionHandler) List(c *gin.Context) {
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
		transcriptionFail(c, err)
		return
	}
	page := application.TranscriptionPage{Items: jobs}
	if len(jobs) > limit {
		page.Items = jobs[:limit]
		raw, err := json.Marshal(cursor{ID: page.Items[limit-1].ID, Binding: binding})
		if err != nil {
			transcriptionFail(c, err)
			return
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		page.NextCursor = &next
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, page)
}

// Get returns current durable facts without private original keys.
// @Summary Get local speech transcription
// @ID getMediaTranscription
// @Tags media-transcriptions
// @Produce json
// @Param job_id path string true "Transcription UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {object} TranscriptionJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-transcriptions/{job_id} [get]
func (h *TranscriptionHandler) Get(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	job, err := h.store.Get(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		transcriptionFail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, job)
}

// Cancel accepts cancellation; cancelled status requires worker cessation evidence.
// @Summary Cancel local speech transcription
// @ID cancelMediaTranscription
// @Tags media-transcriptions
// @Accept json
// @Produce json
// @Param job_id path string true "Transcription UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Current job revision"
// @Success 202 {object} TranscriptionJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-transcriptions/{job_id}/cancel [post]
func (h *TranscriptionHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Retry queues another fenced attempt using immutable source inputs.
// @Summary Retry failed or cancelled local speech transcription
// @ID retryMediaTranscription
// @Tags media-transcriptions
// @Accept json
// @Produce json
// @Param job_id path string true "Transcription UUID"
// @Param Idempotency-Key header string true "Command UUID"
// @Param body body application.ControlInput true "Current job revision"
// @Success 202 {object} TranscriptionJobResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-transcriptions/{job_id}/retry [post]
func (h *TranscriptionHandler) Retry(c *gin.Context) { h.control(c, "retry") }
func (h *TranscriptionHandler) control(c *gin.Context, action string) {
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
		transcriptionFail(c, err)
		return
	}
	c.JSON(202, job)
}

// Result returns machine-recognized cues bound to the frozen reviewed original.
// @Summary Get local speech subtitle draft
// @ID getMediaTranscriptionResult
// @Tags media-transcriptions
// @Produce json
// @Param job_id path string true "Transcription UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {object} application.TranscriptionResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-transcriptions/{job_id}/result [get]
func (h *TranscriptionHandler) Result(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	result, err := h.store.Result(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		transcriptionFail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Subtitles downloads validated native cues without claiming human accuracy review.
// @Summary Download recognized subtitle draft as SRT
// @ID downloadMediaTranscriptionSubtitles
// @Tags media-transcriptions
// @Produce application/x-subrip
// @Param job_id path string true "Transcription UUID"
// @Param project_id query string true "Project UUID"
// @Success 200 {file} binary
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media-transcriptions/{job_id}/subtitles [get]
func (h *TranscriptionHandler) Subtitles(c *gin.Context) {
	project, id, ok := jobScope(c)
	if !ok {
		return
	}
	result, err := h.store.Result(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		transcriptionFail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", "attachment; filename=transcription-"+id.String()+".srt")
	c.Data(200, "application/x-subrip; charset=utf-8", []byte(result.SRT))
}
