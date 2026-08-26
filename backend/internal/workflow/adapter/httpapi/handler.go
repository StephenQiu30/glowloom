package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type MutationService interface {
	Start(context.Context, application.Actor, application.StartCommand) (domain.WorkflowRun, error)
	Rerun(context.Context, application.Actor, application.RerunCommand) (domain.WorkflowRun, error)
}

type QueryService interface {
	GetRun(context.Context, application.Actor, string) (application.RunView, error)
}

type ControlService interface {
	Pause(context.Context, application.Actor, application.PauseCommand) (domain.ControlIntent, error)
	Resume(context.Context, application.Actor, application.ResumeCommand) (domain.ControlIntent, error)
	Cancel(context.Context, application.Actor, application.CancelCommand) (domain.ControlIntent, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	mutations     MutationService
	queries       QueryService
	controls      ControlService
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func New(
	mutations MutationService,
	queries QueryService,
	controls ControlService,
	authenticator Authenticator,
) *Handler {
	return &Handler{
		mutations: mutations, queries: queries, controls: controls,
		authenticator: authenticator, validator: platformvalidation.New(),
	}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflow-runs", handler.start)
	mux.HandleFunc("GET /api/v1/workflow-runs/{run_id}", handler.getRun)
	mux.HandleFunc("POST /api/v1/workflow-runs/{run_id}/reruns", handler.rerun)
	mux.HandleFunc("POST /api/v1/workflow-runs/{run_id}/controls", handler.control)
}

type startRequest struct {
	AuthoringRevisionID string `json:"authoring_revision_id" validate:"required,uuid"`
	IdempotencyKey      string `json:"idempotency_key" validate:"required,max=200"`
}

type rerunRequest struct {
	RootNodeID     string `json:"root_node_id" validate:"required,max=100"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
}

type controlRequest struct {
	Action           string `json:"action" validate:"required,oneof=pause resume cancel"`
	ExpectedRevision int    `json:"expected_revision" validate:"gte=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) start(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload startRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	run, err := handler.mutations.Start(request.Context(), actor, application.StartCommand{
		AuthoringRevisionID: payload.AuthoringRevisionID,
		IdempotencyKey:      payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	handler.respondRun(writer, request, actor, run.ID, http.StatusAccepted)
}

func (handler *Handler) getRun(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	handler.respondRun(writer, request, actor, request.PathValue("run_id"), http.StatusOK)
}

func (handler *Handler) rerun(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload rerunRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	run, err := handler.mutations.Rerun(request.Context(), actor, application.RerunCommand{
		SourceWorkflowRunID: request.PathValue("run_id"),
		RootNodeID:          payload.RootNodeID,
		IdempotencyKey:      payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	handler.respondRun(writer, request, actor, run.ID, http.StatusAccepted)
}

func (handler *Handler) control(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload controlRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	command := application.ControlCommand{
		WorkflowRunID:    request.PathValue("run_id"),
		ExpectedRevision: payload.ExpectedRevision,
		IdempotencyKey:   payload.IdempotencyKey,
	}
	var (
		intent domain.ControlIntent
		err    error
	)
	switch payload.Action {
	case domain.ControlActionPause:
		intent, err = handler.controls.Pause(request.Context(), actor, command)
	case domain.ControlActionResume:
		intent, err = handler.controls.Resume(request.Context(), actor, command)
	case domain.ControlActionCancel:
		intent, err = handler.controls.Cancel(request.Context(), actor, command)
	}
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	view, err := handler.queries.GetRun(request.Context(), actor, command.WorkflowRunID)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	data := presentRunView(view)
	data["control"] = presentControl(intent)
	platformhttp.WriteJSON(writer, http.StatusAccepted, map[string]any{"data": data})
}

func (handler *Handler) respondRun(
	writer http.ResponseWriter,
	request *http.Request,
	actor application.Actor,
	runID string,
	status int,
) {
	view, err := handler.queries.GetRun(request.Context(), actor, runID)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": presentRunView(view)})
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &application.Error{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: http.StatusInternalServerError}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{
		Code: apiError.Code, Message: apiError.Message, Status: apiError.Status,
		NextAction: apiError.NextAction,
	})
}

func presentRunView(view application.RunView) map[string]any {
	nodes := make([]map[string]any, len(view.Nodes))
	for index, node := range view.Nodes {
		nodes[index] = presentNodeRun(node)
	}
	return map[string]any{"run": presentRun(view.Run), "nodes": nodes}
}

func presentRun(value domain.WorkflowRun) map[string]any {
	return map[string]any{
		"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID,
		"authoring_revision_id": value.AuthoringRevisionID,
		"definition_version_id": value.DefinitionVersionID, "run_input_snapshot_id": value.RunInputSnapshotID,
		"temporal_workflow_id": value.TemporalWorkflowID, "start_input_hash": value.StartInputHash,
		"source_workflow_run_id": value.SourceWorkflowRunID, "rerun_root_node_id": value.RerunRootNodeID,
		"status": value.Status, "progress_stage": value.ProgressStage,
		"next_action": value.NextAction, "error": nullableJSON(value.Error),
		"paused_from_status": value.PausedFromStatus, "paused_from_progress_stage": value.PausedFromProgressStage,
		"revision": value.Revision, "created_by": value.CreatedBy,
		"created_at": value.CreatedAt, "updated_at": value.UpdatedAt,
	}
}

func presentNodeRun(value domain.NodeRunProjection) map[string]any {
	return map[string]any{
		"id": value.ID, "workspace_id": value.WorkspaceID, "workflow_run_id": value.WorkflowRunID,
		"node_id": value.NodeID, "definition_key": value.DefinitionKey,
		"definition_version": value.DefinitionVersion, "executor": value.Executor, "risk_level": value.RiskLevel,
		"status": value.Status, "attempt": value.Attempt, "reused_from_node_run_id": value.ReusedFromNodeRunID,
		"input_hash": value.InputHash, "cache_key": value.CacheKey, "output_hash": value.OutputHash,
		"revision": value.Revision, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt,
	}
}

func presentControl(value domain.ControlIntent) map[string]any {
	return map[string]any{
		"id": value.ID, "workflow_run_id": value.WorkflowRunID, "action": value.Action,
		"expected_run_revision": value.ExpectedRunRevision, "status": value.Status,
		"attempt_no": value.AttemptNo, "revision": value.Revision,
		"created_at": value.CreatedAt, "updated_at": value.UpdatedAt,
	}
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
