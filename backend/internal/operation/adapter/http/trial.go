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
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// TrialClient is the workflow API required by administrator experiments.
type TrialClient interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error)
	QueryWorkflow(context.Context, string, string, string, ...any) (converter.EncodedValue, error)
	SignalWorkflow(context.Context, string, string, string, any) error
}

// TrialHandler exposes owner-scoped local administrator experiments.
type TrialHandler struct {
	storageID string
	client    TrialClient
	auth      trial.Authorizer
	objects   *objectstorage.Client
}

// NewTrialHandler injects execution, authority and private result storage.
func NewTrialHandler(c TrialClient, a trial.Authorizer, o *objectstorage.Client, storageID string) *TrialHandler {
	return &TrialHandler{client: c, auth: a, objects: o, storageID: storageID}
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
	value, err := h.client.QueryWorkflow(ctx, application.TrialWorkflowID(identityhttp.Principal(c), id), "", "state")
	var state application.TrialState
	if err == nil {
		err = value.Get(&state)
	}
	if err != nil {
		var missing *serviceerror.NotFound
		if errors.As(err, &missing) {
			httpapi.WriteProblem(c, 404, "trial_not_found", nil)
		} else {
			httpapi.WriteProblem(c, 503, "trial_state_unavailable", nil)
		}
		return state, id, false
	}
	if state.State == "unknown" {
		v, e := h.client.QueryWorkflow(ctx, application.TrialWorkflowID(identityhttp.Principal(c), id)+"/recovery", "", "state")
		if e == nil {
			var recovered application.TrialState
			if v.Get(&recovered) == nil {
				state = recovered
			}
		}
	}
	return state, id, true
}
func publicTrialState(state application.TrialState) application.TrialState {
	turns := append([]application.TrialTurn(nil), state.Turns...)
	for i := range turns {
		turns[i].ThreadID = ""
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
	_, err := h.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: application.TrialWorkflowID(actor, req.RequestID), TaskQueue: "flow", WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionTimeout: 25 * time.Hour}, operationflow.TrialWorkflow, application.TrialInput{Request: req, Actor: actor})
	if err != nil {
		var exists *serviceerror.WorkflowExecutionAlreadyStarted
		if !errors.As(err, &exists) {
			httpapi.WriteProblem(c, 503, "trial_start_unconfirmed", nil)
			return
		}
		c.Params = append(c.Params, gin.Param{Key: "id", Value: req.RequestID.String()})
		state, _, ok := h.read(c)
		if !ok {
			return
		}
		if state.Fingerprint != req.Fingerprint() {
			httpapi.WriteProblem(c, 409, "trial_request_conflict", nil)
			return
		}
		c.JSON(202, publicTrialState(state))
		return
	}
	c.JSON(202, application.TrialState{Kind: req.Kind, Model: req.Model, State: "running", Turns: []application.TrialTurn{}})
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
	if err := h.client.SignalWorkflow(c.Request.Context(), application.TrialWorkflowID(identityhttp.Principal(c), id), "", "turn", message); err != nil {
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
	workflowID := application.TrialWorkflowID(identityhttp.Principal(c), id)
	err := h.client.SignalWorkflow(c.Request.Context(), workflowID, "", "cancel", true)
	var missing *serviceerror.NotFound
	if errors.As(err, &missing) {
		err = h.client.SignalWorkflow(c.Request.Context(), workflowID+"/recovery", "", "cancel", true)
	}
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
	if a.StorageID != h.storageID {
		httpapi.WriteProblem(c, 503, "trial_storage_changed", nil)
		return
	}
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
	last := state.Turns[len(state.Turns)-1]
	input := application.TrialInput{Request: application.TrialRequest{RequestID: id, Source: state.Source, Kind: state.Kind, Model: state.Model, ProviderID: state.ProviderID, Prompt: last.Prompt}, Actor: identityhttp.Principal(c), Action: "recover", TaskID: last.TaskID, ConfigVersionID: last.ConfigVersionID, CredentialID: last.CredentialID, OriginalFingerprint: state.Fingerprint}
	if state.Kind != "image" && state.Kind != "video" {
		httpapi.WriteProblem(c, 409, "trial_recovery_unavailable", nil)
		return
	}
	_, err := h.client.ExecuteWorkflow(c.Request.Context(), client.StartWorkflowOptions{ID: application.TrialWorkflowID(input.Actor, id) + "/recovery", TaskQueue: "flow", WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE, WorkflowExecutionTimeout: 30 * time.Minute}, operationflow.TrialWorkflow, input)
	if err != nil {
		var exists *serviceerror.WorkflowExecutionAlreadyStarted
		if !errors.As(err, &exists) {
			httpapi.WriteProblem(c, 503, "trial_recovery_unavailable", nil)
			return
		}
	}
	c.JSON(202, publicTrialState(state))
}
