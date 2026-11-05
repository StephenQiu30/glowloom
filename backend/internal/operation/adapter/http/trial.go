package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// TrialClient exposes only local administrator trial operations.
type TrialClient interface {
	Start(context.Context, application.TrialInput) (application.TrialState, error)
	State(context.Context, string) (application.TrialState, error)
	Turn(context.Context, string, application.TrialMessage) error
	Cancel(context.Context, string) error
	Recover(context.Context, string) error
}

// TrialHandler exposes owner-scoped local administrator experiments.
type TrialHandler struct {
	client  TrialClient
	auth    trial.Authorizer
	objects trial.Results
}

// NewTrialHandler injects execution, authority and private result storage.
func NewTrialHandler(c TrialClient, a trial.Authorizer, o trial.Results) *TrialHandler {
	return &TrialHandler{client: c, auth: a, objects: o}
}

// Register applies current administrator authorization to every trial route.
func (h *TrialHandler) Register(group *gin.RouterGroup) {
	g := group.Group("/admin/ai-trials")
	g.Use(func(c *gin.Context) {
		actor := identityhttp.Principal(c)
		if actor.PlatformRole != "platform_admin" || h.auth.AuthorizeDevelopmentTrial(c.Request.Context(), actor) != nil {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			c.Abort()
			return
		}
		c.Header("Cache-Control", "private, no-store")
		if h.client == nil {
			httpapi.WriteProblem(c, 503, "trial_runtime_unavailable", nil)
			c.Abort()
		}
	})
	g.POST("", h.Start)
	g.GET("/:id", h.State)
	g.POST("/:id/turns", h.Turn)
	g.POST("/:id/cancel", h.Cancel)
	g.POST("/:id/recover", h.Recover)
	g.GET("/:id/artifacts/:sequence", h.Artifact)
}
func trialBody(c *gin.Context, target any) bool {
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		httpapi.WriteProblem(c, 422, "invalid_trial", nil)
		return false
	}
	return true
}
func (h *TrialHandler) read(c *gin.Context) (application.TrialState, uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_trial", nil)
		return application.TrialState{}, id, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	state, err := h.client.State(ctx, application.TrialID(identityhttp.Principal(c), id))
	if err != nil {
		if errors.Is(err, trialtask.ErrMissing) {
			httpapi.WriteProblem(c, 404, "trial_not_found", nil)
		} else {
			httpapi.WriteProblem(c, 503, "trial_state_unavailable", nil)
		}
		return state, id, false
	}
	return state, id, true
}
func publicTrialState(state application.TrialState) application.TrialState {
	turns := append([]application.TrialTurn(nil), state.Turns...)
	for i := range turns {
		turns[i].ThreadID = ""
		turns[i].Artifacts = append([]application.TrialArtifact(nil), turns[i].Artifacts...)
		for j := range turns[i].Artifacts {
			turns[i].Artifacts[j].ObjectRef = nil
		}
	}
	state.Turns = turns
	return state
}

// Start starts a local administrator trial without model publication or billing.
// @ID startAiTrial
// @Summary Start development AI trial
// @Tags settings
// @Accept json
// @Produce json
// @Param body body application.TrialRequest true "Explicit trial"
// @Success 202 {object} application.TrialState
// @Router /api/admin/ai-trials [post]
func (h *TrialHandler) Start(c *gin.Context) {
	var req application.TrialRequest
	if !trialBody(c, &req) {
		return
	}
	if req.Validate() != nil {
		httpapi.WriteProblem(c, 422, "invalid_trial", nil)
		return
	}
	actor := identityhttp.Principal(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	state, err := h.client.Start(ctx, application.TrialInput{Request: req, Actor: actor})
	if errors.Is(err, trialtask.ErrConflict) {
		httpapi.WriteProblem(c, 409, "trial_request_conflict", nil)
		return
	}
	if err != nil {
		httpapi.WriteProblem(c, 503, "trial_start_unconfirmed", nil)
		return
	}
	c.JSON(202, publicTrialState(state))
}

// State returns durable progress and final responses, without upstream thread identities.
// @ID getAiTrial
// @Summary Read development AI trial
// @Tags settings
// @Produce json
// @Param id path string true "Trial request ID"
// @Success 200 {object} application.TrialState
// @Router /api/admin/ai-trials/{id} [get]
func (h *TrialHandler) State(c *gin.Context) {
	s, _, ok := h.read(c)
	if ok {
		c.JSON(200, publicTrialState(s))
	}
}

// Turn queues one idempotent chat turn on the original private session.
// @ID continueAiTrial
// @Summary Continue development Codex chat
// @Tags settings
// @Accept json
// @Produce json
// @Param id path string true "Trial request ID"
// @Param body body application.TrialMessage true "Chat turn"
// @Success 202 {object} application.TrialState
// @Router /api/admin/ai-trials/{id}/turns [post]
func (h *TrialHandler) Turn(c *gin.Context) {
	var message application.TrialMessage
	if !trialBody(c, &message) {
		return
	}
	state, id, ok := h.read(c)
	if !ok {
		return
	}
	candidate := application.TrialRequest{RequestID: message.RequestID, Source: "codex", Kind: "chat", Model: state.Model, Prompt: message.Prompt}
	if candidate.Validate() != nil || state.Kind != "chat" {
		httpapi.WriteProblem(c, 422, "invalid_trial", nil)
		return
	}
	for _, previous := range state.Turns {
		if previous.RequestID == message.RequestID {
			if previous.Prompt != message.Prompt {
				httpapi.WriteProblem(c, 409, "trial_request_conflict", nil)
				return
			}
			c.JSON(202, publicTrialState(state))
			return
		}
	}
	if state.State != "succeeded" || len(state.Turns) >= 32 {
		httpapi.WriteProblem(c, 409, "trial_session_unavailable", nil)
		return
	}
	if err := h.client.Turn(c.Request.Context(), application.TrialID(identityhttp.Principal(c), id), message); err != nil {
		if errors.Is(err, trialtask.ErrConflict) {
			httpapi.WriteProblem(c, 409, "trial_session_unavailable", nil)
			return
		}
		httpapi.WriteProblem(c, 503, "trial_turn_unconfirmed", nil)
		return
	}
	c.JSON(202, publicTrialState(state))
}

// Cancel requests termination without claiming an upstream generation was cancelled.
// @ID cancelAiTrial
// @Summary Stop development AI trial
// @Tags settings
// @Param id path string true "Trial request ID"
// @Success 204
// @Router /api/admin/ai-trials/{id}/cancel [post]
func (h *TrialHandler) Cancel(c *gin.Context) {
	_, id, ok := h.read(c)
	if !ok {
		return
	}
	err := h.client.Cancel(c.Request.Context(), application.TrialID(identityhttp.Principal(c), id))
	if err != nil {
		httpapi.WriteProblem(c, 503, "trial_cancel_unconfirmed", nil)
		return
	}
	c.Status(204)
}

// Artifact serves only a completed object recorded in the owner's durable trial.
// @ID getAiTrialArtifact
// @Summary Read development AI artifact
// @Tags settings
// @Produce application/octet-stream
// @Param id path string true "Trial request ID"
// @Param sequence path int true "Artifact sequence"
// @Success 200 {file} binary
// @Router /api/admin/ai-trials/{id}/artifacts/{sequence} [get]
func (h *TrialHandler) Artifact(c *gin.Context) {
	state, id, ok := h.read(c)
	if !ok {
		return
	}
	if c.Param("sequence") != "1" || len(state.Turns) == 0 || state.State != "succeeded" {
		httpapi.WriteProblem(c, 404, "trial_artifact_missing", nil)
		return
	}
	artifacts := state.Turns[len(state.Turns)-1].Artifacts
	if len(artifacts) != 1 {
		httpapi.WriteProblem(c, 404, "trial_artifact_missing", nil)
		return
	}
	a := artifacts[0]
	reader, err := trial.ReadArtifact(c.Request.Context(), h.objects, application.TrialInput{Actor: identityhttp.Principal(c), Request: application.TrialRequest{RequestID: id}}, a)
	if err != nil {
		httpapi.WriteProblem(c, 503, "trial_artifact_unavailable", nil)
		return
	}
	defer func() { _ = reader.Close() }()
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(200, a.Size, a.MIME, reader, nil)
}

// Recover checks saved output or the original video task, never submits generation.
// @ID recoverAiTrial
// @Summary Recover development AI result
// @Tags settings
// @Param id path string true "Trial request ID"
// @Success 202 {object} application.TrialState
// @Router /api/admin/ai-trials/{id}/recover [post]
func (h *TrialHandler) Recover(c *gin.Context) {
	state, id, ok := h.read(c)
	if !ok {
		return
	}
	if state.State != "unknown" || len(state.Turns) == 0 {
		httpapi.WriteProblem(c, 409, "trial_recovery_unavailable", nil)
		return
	}
	if err := h.client.Recover(c.Request.Context(), application.TrialID(identityhttp.Principal(c), id)); err != nil {
		if errors.Is(err, trialtask.ErrConflict) {
			httpapi.WriteProblem(c, 409, "trial_recovery_unavailable", nil)
			return
		}
		httpapi.WriteProblem(c, 503, "trial_recovery_unavailable", nil)
		return
	}
	c.JSON(202, publicTrialState(state))
}
