package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// VoiceHandler exposes scope-authorized durable generation, not direct supplier I/O.
type VoiceHandler struct{ jobs application.VoiceJobs }

// NewVoiceHandler injects the actual job owner; construction starts no generation.
func NewVoiceHandler(jobs application.VoiceJobs) *VoiceHandler { return &VoiceHandler{jobs: jobs} }

// Register retains the existing authenticated session, Origin and CSRF boundary.
func (h *VoiceHandler) Register(group *gin.RouterGroup) {
	group.GET("/subjects/voice-options", h.Options)
	group.POST("/subjects/voice-jobs", h.Create)
	group.GET("/subjects/voice-jobs/:job_id", h.Get)
	group.GET("/subject-voice-commands/:key", h.Recover)
}

func voiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidVoiceJob):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, application.ErrVoiceJobNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrVoiceJobKeyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrVoiceUnavailable):
		httpapi.WriteProblem(c, 503, "voice_generation_unavailable", nil)
	default:
		writeError(c, err)
	}
}

func (h *VoiceHandler) ready(c *gin.Context) bool {
	if h == nil || h.jobs == nil {
		voiceError(c, application.ErrVoiceUnavailable)
		return false
	}
	return true
}

// Options lists only formally configured and enabled speech releases.
// @Summary 智能配音模型与音色
// @ID listSubjectVoiceOptions
// @Tags subjects
// @Produce json
// @Success 200 {object} application.VoiceOptions
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/subjects/voice-options [get]
func (h *VoiceHandler) Options(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		voiceError(c, application.ErrInvalidVoiceJob)
		return
	}
	result, err := h.jobs.Options(c.Request.Context(), currentActor(c))
	if err != nil {
		voiceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Create admits one permanent explicit intent. Replay never repeats paid submission.
// @Summary 创建智能配音任务
// @ID createSubjectVoiceJob
// @Tags subjects
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID永久幂等键"
// @Param request body application.CreateVoiceJobInput true "固定草稿意图、正式release与显式音色；文本1..500标量"
// @Success 202 {object} application.VoiceJob
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/subjects/voice-jobs [post]
func (h *VoiceHandler) Create(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		voiceError(c, application.ErrInvalidVoiceJob)
		return
	}
	key, ok := commandKey(c)
	if !ok {
		return
	}
	var input application.CreateVoiceJobInput
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.jobs.Create(c.Request.Context(), currentActor(c), key, input)
	if err != nil {
		voiceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, result)
}

// Get recovers only an explicitly identified job for its actor, scope and draft.
// @Summary 读取智能配音任务
// @ID getSubjectVoiceJob
// @Tags subjects
// @Produce json
// @Param job_id path string true "任务UUID"
// @Success 200 {object} application.VoiceJob
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Router /api/subjects/voice-jobs/{job_id} [get]
func (h *VoiceHandler) Get(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		voiceError(c, application.ErrInvalidVoiceJob)
		return
	}
	id, ok := pathID(c, "job_id")
	if !ok {
		return
	}
	result, err := h.jobs.Get(c.Request.Context(), currentActor(c), id)
	if err != nil {
		voiceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Recover discovers the exact command after transport loss without a new request key.
// @Summary 查证智能配音永久命令
// @ID getSubjectVoiceCommand
// @Tags subjects
// @Produce json
// @Param key path string true "永久命令UUID"
// @Success 200 {object} application.VoiceRecovery
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/subject-voice-commands/{key} [get]
func (h *VoiceHandler) Recover(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		voiceError(c, application.ErrInvalidVoiceJob)
		return
	}
	key, ok := pathID(c, "key")
	if !ok || key == uuid.Nil {
		return
	}
	result, err := h.jobs.Recover(c.Request.Context(), currentActor(c), key)
	if err != nil {
		voiceError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}
