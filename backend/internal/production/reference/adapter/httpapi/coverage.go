package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	referenceapp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/application"
)

type ReferenceCoverageQuery interface {
	GetMatrix(context.Context, referenceapp.Actor, string) (referenceapp.ReferenceCoverageMatrix, error)
	GetTarget(context.Context, referenceapp.Actor, string, string) (referenceapp.ReferenceTargetDetail, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type ReferenceCoverageHandler struct {
	query         ReferenceCoverageQuery
	authenticator Authenticator
}

func NewReferenceCoverageHandler(
	query ReferenceCoverageQuery,
	authenticator Authenticator,
) *ReferenceCoverageHandler {
	return &ReferenceCoverageHandler{query: query, authenticator: authenticator}
}

func (handler *ReferenceCoverageHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/reference-coverage", handler.getMatrix)
	mux.HandleFunc("GET /api/projects/{project_id}/reference-targets/{target_version_id}", handler.getTarget)
}

func (handler *ReferenceCoverageHandler) getMatrix(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.readActor(writer, request)
	if !ok {
		return
	}
	result, err := handler.query.GetMatrix(request.Context(), actor, request.PathValue("project_id"))
	handler.writeResult(writer, request, result, err)
}

func (handler *ReferenceCoverageHandler) getTarget(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.readActor(writer, request)
	if !ok {
		return
	}
	result, err := handler.query.GetTarget(
		request.Context(), actor, request.PathValue("project_id"), request.PathValue("target_version_id"),
	)
	handler.writeResult(writer, request, result, err)
}

func (handler *ReferenceCoverageHandler) readActor(
	writer http.ResponseWriter,
	request *http.Request,
) (referenceapp.Actor, bool) {
	writer.Header().Set("Cache-Control", "no-store")
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &referenceapp.QueryError{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized,
		})
		return referenceapp.Actor{}, false
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil || len(body) != 0 || request.URL.RawQuery != "" || request.URL.ForceQuery {
		handler.writeError(writer, request, &referenceapp.QueryError{
			Code: "validation_failed", Message: "Reference query does not accept a body or selector", Status: http.StatusUnprocessableEntity,
		})
		return referenceapp.Actor{}, false
	}
	return referenceapp.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *ReferenceCoverageHandler) writeResult(
	writer http.ResponseWriter,
	request *http.Request,
	result any,
	err error,
) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": result})
}

func (handler *ReferenceCoverageHandler) writeError(
	writer http.ResponseWriter,
	request *http.Request,
	err error,
) {
	var problem *referenceapp.QueryError
	if !errors.As(err, &problem) {
		problem = &referenceapp.QueryError{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{
		Code: problem.Code, Message: problem.Message, Status: problem.Status,
	})
}
