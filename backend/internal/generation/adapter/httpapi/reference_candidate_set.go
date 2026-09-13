package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

type ReferenceCandidateSetOwner interface {
	Materialize(context.Context, application.Actor, application.ReferenceCandidateSetCommand) (domain.ReferenceCandidateSet, error)
	Get(context.Context, application.Actor, string, string) (domain.ReferenceCandidateSet, error)
}

type ReferenceCandidateSetHandler struct {
	owner         ReferenceCandidateSetOwner
	authenticator Authenticator
}

func NewReferenceCandidateSetHandler(owner ReferenceCandidateSetOwner, authenticator Authenticator) *ReferenceCandidateSetHandler {
	return &ReferenceCandidateSetHandler{owner: owner, authenticator: authenticator}
}

func (handler *ReferenceCandidateSetHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{project_id}/reference-executions/{execution_id}/candidate-sets", handler.materialize)
	mux.HandleFunc("GET /api/projects/{project_id}/reference-candidate-sets/{set_id}", handler.get)
}

func (handler *ReferenceCandidateSetHandler) serve(writer http.ResponseWriter, request *http.Request, create bool) {
	writer.Header().Set("Cache-Control", "no-store")
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401})
		return
	}
	actor := application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}
	var value domain.ReferenceCandidateSet
	status := http.StatusOK
	if create {
		var body struct {
			ExecutionHash        string                         `json:"execution_hash"`
			ExpectedProgressHash string                         `json:"expected_progress_hash"`
			BundleRefs           []domain.GenerationRevisionRef `json:"candidate_bundle_refs"`
		}
		raw, readErr := io.ReadAll(http.MaxBytesReader(writer, request.Body, 16*1024))
		if readErr != nil || request.URL.RawQuery != "" || request.URL.ForceQuery || canonical.Decode(raw, &body) != nil {
			writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Invalid candidate Set request", Status: 422})
			return
		}
		value, err = handler.owner.Materialize(request.Context(), actor, application.ReferenceCandidateSetCommand{
			ProjectID: request.PathValue("project_id"), ExecutionRef: domain.GenerationRevisionRef{ID: request.PathValue("execution_id"), Revision: 1, ContentHash: body.ExecutionHash}, ExpectedProgressHash: body.ExpectedProgressHash, BundleRefs: body.BundleRefs,
		})
		status = http.StatusCreated
	} else {
		body, readErr := io.ReadAll(io.LimitReader(request.Body, 1))
		if readErr != nil || len(body) != 0 || request.URL.RawQuery != "" || request.URL.ForceQuery {
			writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Candidate Set query does not accept a body or selector", Status: 422})
			return
		}
		value, err = handler.owner.Get(request.Context(), actor, request.PathValue("project_id"), request.PathValue("set_id"))
	}
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": value})
}

func (handler *ReferenceCandidateSetHandler) materialize(writer http.ResponseWriter, request *http.Request) {
	handler.serve(writer, request, true)
}
func (handler *ReferenceCandidateSetHandler) get(writer http.ResponseWriter, request *http.Request) {
	handler.serve(writer, request, false)
}
