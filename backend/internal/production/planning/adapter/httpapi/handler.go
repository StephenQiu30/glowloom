package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

type Service interface {
	CreatePlan(context.Context, application.Actor, application.CreatePlanCommand) (application.View, error)
	GetPlan(context.Context, application.Actor, string) (application.View, error)
	ConfirmPlan(context.Context, application.Actor, application.ConfirmPlanCommand) (application.View, error)
	Materialize(context.Context, application.Actor, application.MaterializeCommand) (domain.ImportCommit, error)
	Publish(context.Context, application.Actor, application.PublishCommand) (domain.ImportCommit, error)
	ListEpisodes(context.Context, application.Actor, string) ([]domain.Episode, error)
	GetEpisode(context.Context, application.Actor, string) (domain.Episode, error)
	GetEpisodeStructure(context.Context, application.Actor, string) (domain.Structure, error)
	AcceptTask(context.Context, application.Actor, application.AcceptTaskCommand) (domain.Structure, error)
	ConfirmStructure(context.Context, application.Actor, application.StructureCommand) (domain.Structure, error)
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
	mux.HandleFunc("POST /api/v1/document-revisions/{revision_id}/episode-plans", handler.createPlan)
	mux.HandleFunc("GET /api/v1/episode-plans/{plan_id}", handler.getPlan)
	mux.HandleFunc("POST /api/v1/episode-plans/{plan_id}/confirm", handler.confirmPlan)
	mux.HandleFunc("POST /api/v1/episode-plans/{plan_id}/materializations", handler.materialize)
	mux.HandleFunc("POST /api/v1/import-commits/{commit_id}/publish", handler.publish)
	mux.HandleFunc("GET /api/v1/projects/{project_id}/episodes", handler.listEpisodes)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}", handler.getEpisode)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}/structure", handler.getStructure)
	mux.HandleFunc("POST /api/v1/episode-structures/{structure_id}/tasks/{task_id}/accept", handler.acceptTask)
	mux.HandleFunc("POST /api/v1/episode-structures/{structure_id}/confirm", handler.confirmStructure)
}

type createPlanRequest struct {
	Strategy              string `json:"strategy" validate:"required,oneof=explicit_markers target_duration_ai"`
	TargetDurationMS      int    `json:"target_duration_ms" validate:"required,min=15000,max=600000"`
	RequestedEpisodeCount *int   `json:"requested_episode_count" validate:"omitempty,min=1,max=100"`
	IdempotencyKey        string `json:"idempotency_key" validate:"required,max=200"`
}
type revisionRequest struct {
	ExpectedRevision int    `json:"expected_revision" validate:"required,min=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}
type materializeRequest struct {
	Mode                    string `json:"mode" validate:"required,eq=append_new"`
	ExpectedPlanRevision    int    `json:"expected_plan_revision" validate:"required,min=1"`
	ExpectedProjectRevision int    `json:"expected_project_revision" validate:"required,min=1"`
	ExpectedActiveOrderHash string `json:"expected_active_order_hash" validate:"required,len=64,hexadecimal"`
	IdempotencyKey          string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) createPlan(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload createPlanRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.CreatePlan(request.Context(), actor, application.CreatePlanCommand{RevisionID: request.PathValue("revision_id"), Strategy: payload.Strategy, TargetDurationMS: payload.TargetDurationMS, RequestedEpisodeCount: payload.RequestedEpisodeCount, IdempotencyKey: payload.IdempotencyKey})
	handler.writeView(writer, request, http.StatusCreated, result, err)
}
func (handler *Handler) getPlan(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.GetPlan(request.Context(), actor, request.PathValue("plan_id"))
	handler.writeView(writer, request, http.StatusOK, result, err)
}
func (handler *Handler) confirmPlan(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload revisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.ConfirmPlan(request.Context(), actor, application.ConfirmPlanCommand{PlanID: request.PathValue("plan_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	handler.writeView(writer, request, http.StatusOK, result, err)
}
func (handler *Handler) materialize(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload materializeRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Materialize(request.Context(), actor, application.MaterializeCommand{PlanID: request.PathValue("plan_id"), Mode: payload.Mode, ExpectedPlanRevision: payload.ExpectedPlanRevision, ExpectedProjectRevision: payload.ExpectedProjectRevision, ExpectedActiveOrderHash: payload.ExpectedActiveOrderHash, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": presentCommit(result)})
}
func (handler *Handler) publish(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload revisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Publish(request.Context(), actor, application.PublishCommand{CommitID: request.PathValue("commit_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentCommit(result)})
}
func (handler *Handler) listEpisodes(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	values, err := handler.service.ListEpisodes(request.Context(), actor, request.PathValue("project_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	items := make([]map[string]any, len(values))
	for index, value := range values {
		items[index] = presentEpisode(value)
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": items})
}
func (handler *Handler) getEpisode(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetEpisode(request.Context(), actor, request.PathValue("episode_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentEpisode(value)})
}
func (handler *Handler) getStructure(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetEpisodeStructure(request.Context(), actor, request.PathValue("episode_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentStructure(value)})
}
func (handler *Handler) acceptTask(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload revisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.AcceptTask(request.Context(), actor, application.AcceptTaskCommand{StructureCommand: application.StructureCommand{StructureID: request.PathValue("structure_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey}, TaskID: request.PathValue("task_id")})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentStructure(value)})
}
func (handler *Handler) confirmStructure(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload revisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.ConfirmStructure(request.Context(), actor, application.StructureCommand{StructureID: request.PathValue("structure_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentStructure(value)})
}

func (handler *Handler) writeView(writer http.ResponseWriter, request *http.Request, status int, value application.View, err error) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": presentView(value)})
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

func presentView(value application.View) map[string]any {
	blocks := make([]map[string]any, len(value.Plan.Source.Blocks))
	for index, block := range value.Plan.Source.Blocks {
		blocks[index] = map[string]any{"id": block.ID, "document_revision_id": block.DocumentRevisionID, "position": block.Position, "kind": block.Kind, "source_start": block.SourceStart, "source_end": block.SourceEnd, "text_hash": block.TextHash, "metadata": block.Metadata}
	}
	proposals := make([]map[string]any, len(value.Plan.Proposals))
	for index, item := range value.Plan.Proposals {
		proposals[index] = map[string]any{"id": item.ID, "plan_id": item.PlanID, "position": item.Position, "title": item.Title, "start_block_id": item.StartBlockID, "end_block_id": item.EndBlockID, "start_block_position": item.StartBlockPosition, "end_block_position": item.EndBlockPosition, "source_start": item.SourceStart, "source_end": item.SourceEnd, "content_hash": item.ContentHash, "estimated_duration_ms": item.EstimatedDurationMS, "reason": item.Reason, "confidence": item.Confidence, "boundary_evidence": item.BoundaryEvidence, "is_locked": item.IsLocked}
	}
	blockers := make([]map[string]any, len(value.Impact.Blockers))
	for index, item := range value.Impact.Blockers {
		blockers[index] = map[string]any{"code": item.Code, "summary": item.Summary, "next_action": item.NextAction}
	}
	return map[string]any{"plan": map[string]any{"id": value.Plan.ID, "workspace_id": value.Plan.WorkspaceID, "project_id": value.Plan.ProjectID, "document_revision_id": value.Plan.DocumentRevisionID, "strategy": value.Plan.Strategy, "status": value.Plan.Status, "target_duration_ms": value.Plan.TargetDurationMS, "requested_episode_count": value.Plan.RequestedEpisodeCount, "total_estimated_duration_ms": value.Plan.TotalEstimatedDurationMS, "input_hash": value.Plan.InputHash, "planning_engine_version": value.Plan.EngineVersion, "model_name": value.Plan.ModelName, "prompt_version": value.Plan.PromptVersion, "schema_version": value.Plan.SchemaVersion, "planning_task_id": nil, "planning_error_code": value.Plan.PlanningErrorCode, "revision": value.Plan.Revision, "confirmed_by": value.Plan.ConfirmedBy, "confirmed_at": value.Plan.ConfirmedAt, "created_by": value.Plan.CreatedBy, "created_at": value.Plan.CreatedAt, "updated_at": value.Plan.UpdatedAt}, "proposals": proposals, "impact": map[string]any{"project_revision": value.Impact.ProjectRevision, "active_episode_count": value.Impact.ActiveEpisodeCount, "active_order_hash": value.Impact.ActiveOrderHash, "projected_episode_count": value.Impact.ProjectedEpisodeCount, "allowed": value.Impact.Allowed, "blockers": blockers}, "source": map[string]any{"document_revision_id": value.Plan.Source.DocumentRevisionID, "normalized_text": value.Plan.Source.NormalizedText, "normalized_hash": value.Plan.Source.NormalizedHash, "codepoint_count": value.Plan.Source.CodepointCount, "blocks": blocks}}
}
func presentCommit(value domain.ImportCommit) map[string]any {
	segments := make([]map[string]any, len(value.Segments))
	for index, item := range value.Segments {
		segments[index] = map[string]any{"id": item.ID, "import_commit_id": item.ImportCommitID, "proposal_id": item.ProposalID, "document_revision_id": item.DocumentRevisionID, "episode_id": item.EpisodeID, "source_id": item.SourceID, "draft_version_id": item.DraftVersionID, "published_version_id": item.PublishedVersionID, "position": item.Position, "source_start": item.SourceStart, "source_end": item.SourceEnd, "source_hash": item.SourceHash}
	}
	return map[string]any{"commit": map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "plan_id": value.PlanID, "mode": value.Mode, "status": value.Status, "input_hash": value.InputHash, "expected_project_revision": value.ExpectedProjectRevision, "expected_active_order_hash": value.ExpectedActiveOrderHash, "error_code": value.ErrorCode, "revision": value.Revision, "created_by": value.CreatedBy, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}, "segments": segments}
}
func presentEpisode(value domain.Episode) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "name": value.Name, "position": value.Position, "target_duration_ms": value.TargetDurationMS, "status": value.Status, "revision": value.Revision, "current_script_version_id": value.CurrentScriptVersionID, "current_timeline_version_id": value.CurrentTimelineVersionID}
}
func presentStructure(value domain.Structure) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "episode_id": value.EpisodeID, "script_version_id": value.ScriptVersionID, "status": value.Status, "result_hash": value.ResultHash, "revision": value.Revision, "confirmed_by": value.ConfirmedBy, "confirmed_at": value.ConfirmedAt, "scenes": value.Scenes, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
}
