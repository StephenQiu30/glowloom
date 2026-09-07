package httpapi

import (
	"context"
	"net/http"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

type IntentService interface {
	GetDraftSet(context.Context, application.Actor, string) (domain.DraftSet, error)
	GetApprovedIntents(context.Context, application.Actor, string) (application.FreezeIntentSetResult, error)
	FreezeIntentSet(context.Context, application.Actor, application.FreezeIntentSetCommand) (application.FreezeIntentSetResult, error)
}

type IntentHandler struct {
	service       IntentService
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func NewIntentHandler(service IntentService, authenticator Authenticator) *IntentHandler {
	return &IntentHandler{service: service, authenticator: authenticator, validator: platformvalidation.New()}
}

func (handler *IntentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/storyboard-draft-sets/{set_id}", handler.getSet)
	mux.HandleFunc("GET /api/storyboard-draft-sets/{set_id}/approved-intents", handler.getApproved)
	mux.HandleFunc("POST /api/projects/{project_id}/storyboard-intent-acceptances", handler.freeze)
}

type freezeIntentRequest struct {
	WorkspaceID               string `json:"workspace_id" validate:"required,uuid"`
	CandidateRevisionID       string `json:"candidate_revision_id" validate:"required,uuid"`
	CandidateRevisionHash     string `json:"candidate_revision_hash" validate:"required,len=64,hexadecimal"`
	ExpectedCandidateRevision int64  `json:"expected_candidate_revision" validate:"required,min=1"`
	ReviewDecisionID          string `json:"review_decision_id" validate:"required,uuid"`
	IdempotencyKey            string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *IntentHandler) getSet(writer http.ResponseWriter, request *http.Request) {
	actor, ok := authenticateActor(handler.authenticator, writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetDraftSet(request.Context(), actor, request.PathValue("set_id"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentSet(value)})
}
func (handler *IntentHandler) getApproved(writer http.ResponseWriter, request *http.Request) {
	actor, ok := authenticateActor(handler.authenticator, writer, request)
	if !ok {
		return
	}
	value, err := handler.service.GetApprovedIntents(request.Context(), actor, request.PathValue("set_id"))
	handler.writeAccepted(writer, request, value, err)
}
func (handler *IntentHandler) freeze(writer http.ResponseWriter, request *http.Request) {
	actor, ok := authenticateActor(handler.authenticator, writer, request)
	if !ok {
		return
	}
	var payload freezeIntentRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	value, err := handler.service.FreezeIntentSet(request.Context(), actor, application.FreezeIntentSetCommand{
		WorkspaceID: payload.WorkspaceID, ProjectID: request.PathValue("project_id"), CandidateRevisionID: payload.CandidateRevisionID,
		CandidateRevisionHash: payload.CandidateRevisionHash, ExpectedCandidateRevision: payload.ExpectedCandidateRevision,
		ReviewDecisionID: payload.ReviewDecisionID, IdempotencyKey: payload.IdempotencyKey,
	})
	handler.writeAccepted(writer, request, value, err)
}
func (handler *IntentHandler) writeAccepted(writer http.ResponseWriter, request *http.Request, value application.FreezeIntentSetResult, err error) {
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{
		"set": presentSet(value.Set), "approved": value.Approved,
		"receipt": map[string]any{"id": value.Receipt.ID, "operation": value.Receipt.Operation, "resource_id": value.Receipt.ResourceID, "created_at": value.Receipt.CreatedAt},
	}})
}
func presentSet(value domain.DraftSet) map[string]any {
	batches := value.Batches
	if batches == nil {
		batches = []domain.DraftSetBatch{}
	}
	return map[string]any{
		"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID,
		"workflow_run_id": value.WorkflowRunID, "node_run_id": value.NodeRunID, "graph_version_id": value.GraphVersionID,
		"graph_version_no": value.GraphVersionNo, "graph_content_hash": value.GraphContentHash,
		"manifest_id": value.ManifestID, "manifest_version": value.ManifestVersion, "manifest_hash": value.ManifestHash,
		"status": value.Status, "input_hash": value.InputHash, "result_hash": value.ResultHash,
		"candidate_revision_id": value.CandidateRevisionID, "candidate_revision_hash": value.CandidateRevisionHash,
		"batches": batches, "revision": value.Revision, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt,
	}
}
