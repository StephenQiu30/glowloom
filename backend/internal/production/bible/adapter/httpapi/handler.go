package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type Service interface {
	Create(context.Context, application.Actor, application.CreateCommand) (domain.Bible, error)
	Get(context.Context, application.Actor, string) (domain.Bible, error)
	GetCurrent(context.Context, application.Actor, string) (domain.Bible, error)
	Confirm(context.Context, application.Actor, application.ConfirmCommand) (domain.Bible, error)
	DecideReviewIssue(context.Context, application.Actor, application.DecideReviewIssueCommand) (domain.Bible, error)
	Resume(context.Context, application.Actor, application.ResumeCommand) (domain.Bible, error)
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
	mux.HandleFunc("POST /api/v1/document-revisions/{revision_id}/production-bibles", handler.create)
	mux.HandleFunc("GET /api/v1/production-bibles/{bible_id}", handler.get)
	mux.HandleFunc("GET /api/v1/projects/{project_id}/production-bible", handler.getCurrent)
	mux.HandleFunc("POST /api/v1/production-bibles/{bible_id}/confirm", handler.confirm)
	mux.HandleFunc("POST /api/v1/production-bibles/{bible_id}/review-decisions", handler.decideReviewIssue)
	mux.HandleFunc("POST /api/v1/production-bibles/{bible_id}/resume", handler.resume)
}

type createRequest struct {
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
}
type confirmRequest struct {
	ExpectedRevision   int    `json:"expected_revision" validate:"required,min=1"`
	ExpectedResultHash string `json:"expected_result_hash" validate:"required,len=64,hexadecimal"`
	IdempotencyKey     string `json:"idempotency_key" validate:"required,max=200"`
}
type resumeRequest struct {
	ExpectedRevision int    `json:"expected_revision" validate:"required,min=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}
type reviewDecisionRequest struct {
	IssueKey         string `json:"issue_key" validate:"required,max=100"`
	Action           string `json:"action" validate:"required,oneof=accepted rejected"`
	ExpectedRevision int    `json:"expected_revision" validate:"required,min=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) create(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload createRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Create(request.Context(), actor, application.CreateCommand{RevisionID: request.PathValue("revision_id"), IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusAccepted, map[string]any{"data": presentBible(result)})
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.Get(request.Context(), actor, request.PathValue("bible_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentBible(result)})
}

func (handler *Handler) getCurrent(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.GetCurrent(request.Context(), actor, request.PathValue("project_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentBible(result)})
}

func (handler *Handler) confirm(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload confirmRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Confirm(request.Context(), actor, application.ConfirmCommand{BibleID: request.PathValue("bible_id"), ExpectedRevision: payload.ExpectedRevision, ExpectedResultHash: payload.ExpectedResultHash, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentBible(result)})
}

func (handler *Handler) resume(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload resumeRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Resume(request.Context(), actor, application.ResumeCommand{BibleID: request.PathValue("bible_id"), ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusAccepted, map[string]any{"data": presentBible(result)})
}

func (handler *Handler) decideReviewIssue(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload reviewDecisionRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.DecideReviewIssue(request.Context(), actor, application.DecideReviewIssueCommand{
		BibleID: request.PathValue("bible_id"), IssueKey: payload.IssueKey, Action: payload.Action,
		ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentBible(result)})
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

func presentBible(value domain.Bible) map[string]any {
	entities := make([]map[string]any, len(value.Candidate.Entities))
	for index, entity := range value.Candidate.Entities {
		entityID := stableID(value.ID, "entity", entity.EntityKey)
		states := make([]map[string]any, len(entity.States))
		for stateIndex, state := range entity.States {
			states[stateIndex] = map[string]any{"id": stableID(value.ID, "state", entity.EntityKey+":"+state.StateKey), "entity_id": entityID, "state_key": state.StateKey, "label": state.Label, "state_spec": state.StateSpec, "episode_numbers": state.EpisodeNumbers, "evidence": state.Evidence, "asset_state_id": nil, "asset_version_id": nil, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
		}
		entities[index] = map[string]any{"id": entityID, "entity_key": entity.EntityKey, "kind": entity.Kind, "canonical_name": entity.CanonicalName, "normalized_name": entity.NormalizedName, "aliases": entity.Aliases, "stable_spec": entity.StableSpec, "episode_numbers": entity.EpisodeNumbers, "evidence": entity.Evidence, "asset_id": nil, "states": states, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
	}
	worldEntries := make([]map[string]any, len(value.Candidate.WorldEntries))
	for index, entry := range value.Candidate.WorldEntries {
		worldEntries[index] = map[string]any{"id": stableID(value.ID, "world", entry.EntryKey), "entry_key": entry.EntryKey, "category": entry.Category, "title": entry.Title, "facts": entry.Facts, "rules": entry.Rules, "entity_keys": entry.EntityKeys, "episode_numbers": entry.EpisodeNumbers, "evidence": entry.Evidence, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
	}
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "document_revision_id": value.DocumentRevisionID, "task_id": value.TaskID, "status": value.Status, "input_hash": value.InputHash, "result_hash": value.ResultHash, "engine_version": value.EngineVersion, "model_name": value.ModelName, "prompt_version": value.PromptVersion, "schema_version": value.SchemaVersion, "harness_version": value.HarnessVersion, "checkpoint_stage": value.CheckpointStage, "checkpoint_revision": value.CheckpointRevision, "checkpoint_updated_at": value.CheckpointUpdatedAt, "review_issues": value.Candidate.ReviewIssues, "review_decisions": value.ReviewDecisions, "revision": value.Revision, "confirmed_at": value.ConfirmedAt, "confirmed_by": value.ConfirmedBy, "entities": entities, "world_entries": worldEntries, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}
}

func stableID(bibleID, kind, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(bibleID+":"+kind+":"+key)).String()
}
