package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type Service interface {
	CreateBatch(context.Context, application.Actor, application.CreateBatchCommand) (domain.Batch, error)
	GetBatch(context.Context, application.Actor, string) (domain.Batch, error)
	GetLatestBatch(context.Context, application.Actor, string) (domain.Batch, error)
	Decide(context.Context, application.Actor, application.DecisionCommand) (domain.Batch, error)
	Approve(context.Context, application.Actor, application.RevisionCommand) (domain.Batch, error)
	PreflightApply(context.Context, application.Actor, string, int) (application.ApplyPreflight, error)
	Apply(context.Context, application.Actor, application.ApplyCommand) (domain.Batch, []domain.Shot, error)
	ListShots(context.Context, application.Actor, string) ([]domain.Shot, error)
	PreflightExport(context.Context, application.Actor, string) (application.ExportPreflight, error)
	CreateExport(context.Context, application.Actor, application.ExportCommand) (domain.Export, error)
	GetExport(context.Context, application.Actor, string) (domain.Export, error)
	GetLatestExport(context.Context, application.Actor, string) (domain.Export, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}
type Handler struct {
	service       Service
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func New(service Service, authenticator Authenticator) *Handler {
	return &Handler{service: service, authenticator: authenticator, validator: platformvalidation.New()}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/episodes/{episode_id}/storyboard-drafts", handler.createBatch)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}/storyboard-draft", handler.getLatestBatch)
	mux.HandleFunc("GET /api/v1/storyboard-draft-batches/{batch_id}", handler.getBatch)
	mux.HandleFunc("POST /api/v1/storyboard-draft-batches/{batch_id}/decisions", handler.decide)
	mux.HandleFunc("POST /api/v1/storyboard-draft-batches/{batch_id}/approve", handler.approve)
	mux.HandleFunc("POST /api/v1/storyboard-draft-batches/{batch_id}/apply-preflight", handler.preflightApply)
	mux.HandleFunc("POST /api/v1/storyboard-draft-batches/{batch_id}/apply", handler.apply)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}/shots", handler.listShots)
	mux.HandleFunc("POST /api/v1/episodes/{episode_id}/storyboard-exports/preflight", handler.preflightExport)
	mux.HandleFunc("POST /api/v1/episodes/{episode_id}/storyboard-exports", handler.createExport)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}/storyboard-export", handler.getLatestExport)
	mux.HandleFunc("GET /api/v1/storyboard-exports/{export_id}", handler.getExport)
	mux.HandleFunc("GET /api/v1/storyboard-exports/{export_id}/download", handler.downloadExport)
}

type createRequest struct {
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
}
type revisionRequest struct {
	ExpectedRevision int    `json:"expected_revision" validate:"required,min=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}
type decisionRequest struct {
	ProposalKey      string `json:"proposal_key" validate:"required,max=120"`
	Action           string `json:"action" validate:"required,eq=accepted"`
	ExpectedRevision int    `json:"expected_revision" validate:"required,min=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}
type preflightRequest struct {
	ExpectedRevision int `json:"expected_revision" validate:"required,min=1"`
}
type applyRequest struct {
	ExpectedRevision  int    `json:"expected_revision" validate:"required,min=1"`
	ExpectedOrderHash string `json:"expected_order_hash" validate:"required,len=64,hexadecimal"`
	ImpactHash        string `json:"impact_hash" validate:"required,len=64,hexadecimal"`
	IdempotencyKey    string `json:"idempotency_key" validate:"required,max=200"`
}
type exportRequest struct {
	ExpectedOrderHash string `json:"expected_order_hash" validate:"required,len=64,hexadecimal"`
	IdempotencyKey    string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) createBatch(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload createRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.CreateBatch(request.Context(), actor, application.CreateBatchCommand{EpisodeID: request.PathValue("episode_id"), IdempotencyKey: payload.IdempotencyKey})
	handler.writeBatch(writer, request, http.StatusAccepted, value, err)
}
func (handler *Handler) getLatestBatch(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetLatestBatch(request.Context(), actor, request.PathValue("episode_id"))
	handler.writeBatch(writer, request, http.StatusOK, value, err)
}
func (handler *Handler) getBatch(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetBatch(request.Context(), actor, request.PathValue("batch_id"))
	handler.writeBatch(writer, request, http.StatusOK, value, err)
}
func (handler *Handler) decide(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload decisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.Decide(request.Context(), actor, application.DecisionCommand{BatchID: request.PathValue("batch_id"), ProposalKey: payload.ProposalKey, Action: payload.Action, ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	handler.writeBatch(writer, request, http.StatusOK, value, err)
}
func (handler *Handler) approve(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload revisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.Approve(request.Context(), actor, application.RevisionCommand{BatchID: request.PathValue("batch_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	handler.writeBatch(writer, request, http.StatusOK, value, err)
}
func (handler *Handler) preflightApply(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload preflightRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.PreflightApply(request.Context(), actor, request.PathValue("batch_id"), payload.ExpectedRevision)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"batch_id": value.BatchID, "batch_revision": value.BatchRevision, "order_hash": value.OrderHash, "impact_hash": value.ImpactHash, "created": value.Created}})
}
func (handler *Handler) apply(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload applyRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	batch, shots, err := handler.service.Apply(request.Context(), actor, application.ApplyCommand{RevisionCommand: application.RevisionCommand{BatchID: request.PathValue("batch_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey}, ExpectedOrderHash: payload.ExpectedOrderHash, ImpactHash: payload.ImpactHash})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"batch": presentBatch(batch), "shots": presentShots(shots)}})
}
func (handler *Handler) listShots(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	values, err := handler.service.ListShots(request.Context(), actor, request.PathValue("episode_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentShots(values)})
}
func (handler *Handler) preflightExport(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.PreflightExport(request.Context(), actor, request.PathValue("episode_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"episode_id": value.EpisodeID, "order_hash": value.OrderHash, "allowed": value.Allowed, "shot_count": value.ShotCount, "blockers": value.Blockers}})
}
func (handler *Handler) createExport(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload exportRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.CreateExport(request.Context(), actor, application.ExportCommand{EpisodeID: request.PathValue("episode_id"), ExpectedOrderHash: payload.ExpectedOrderHash, IdempotencyKey: payload.IdempotencyKey})
	handler.writeExport(writer, request, http.StatusCreated, value, err)
}
func (handler *Handler) getLatestExport(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetLatestExport(request.Context(), actor, request.PathValue("episode_id"))
	handler.writeExport(writer, request, http.StatusOK, value, err)
}
func (handler *Handler) getExport(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetExport(request.Context(), actor, request.PathValue("export_id"))
	handler.writeExport(writer, request, http.StatusOK, value, err)
}
func (handler *Handler) downloadExport(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetExport(request.Context(), actor, request.PathValue("export_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	writer.Header().Set("Content-Type", "application/zip")
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=storyboard-%s.zip", value.EpisodeID))
	writer.Header().Set("ETag", `"`+value.ContentHash+`"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(value.Package)
}

func (handler *Handler) writeBatch(writer http.ResponseWriter, request *http.Request, status int, value domain.Batch, err error) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": presentBatch(value)})
}
func (handler *Handler) writeExport(writer http.ResponseWriter, request *http.Request, status int, value domain.Export, err error) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": presentExport(value)})
}
func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}
func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: apiError.Code, Message: apiError.Message, Status: apiError.Status, NextAction: apiError.NextAction, Details: apiError.Details})
}

func presentBatch(value domain.Batch) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "episode_id": value.EpisodeID, "structure_id": value.StructureID, "script_version_id": value.ScriptVersionID, "task_id": value.TaskID, "status": value.Status, "input_hash": value.InputHash, "result_hash": value.ResultHash, "candidate": value.Candidate, "decisions": value.Decisions, "error": value.Error, "revision": value.Revision, "approved_by": value.ApprovedBy, "approved_at": value.ApprovedAt, "applied_at": value.AppliedAt, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
}
func presentShots(values []domain.Shot) []map[string]any {
	items := make([]map[string]any, len(values))
	for index, value := range values {
		items[index] = map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "episode_id": value.EpisodeID, "batch_id": value.BatchID, "proposal_key": value.ProposalKey, "position": value.Position, "title": value.Title, "narrative_unit_ids": value.NarrativeUnitIDs, "spec": value.Spec, "content_hash": value.ContentHash, "status": value.Status, "revision": value.Revision, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
	}
	return items
}
func presentExport(value domain.Export) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "episode_id": value.EpisodeID, "status": value.Status, "input_hash": value.InputHash, "content_hash": value.ContentHash, "manifest": value.Manifest, "files": value.Files, "revision": value.Revision, "download_url": "/api/v1/storyboard-exports/" + value.ID + "/download", "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
}
