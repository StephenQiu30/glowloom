package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

type ReferenceExecutionQuery interface {
	Get(context.Context, application.Actor, string, string) (domain.ReferenceJobProgress, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type ReferenceExecutionHandler struct {
	query         ReferenceExecutionQuery
	authenticator Authenticator
}

func NewReferenceExecutionHandler(query ReferenceExecutionQuery, authenticator Authenticator) *ReferenceExecutionHandler {
	return &ReferenceExecutionHandler{query: query, authenticator: authenticator}
}

func (handler *ReferenceExecutionHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/reference-executions/{execution_id}", handler.get)
}

func (handler *ReferenceExecutionHandler) get(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401})
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil || len(body) != 0 || request.URL.RawQuery != "" || request.URL.ForceQuery {
		writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Reference execution query does not accept a body or selector", Status: 422})
		return
	}
	value, err := handler.query.Get(request.Context(), application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, request.PathValue("project_id"), request.PathValue("execution_id"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": value})
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var problem *application.Error
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		problem = &application.Error{Code: "resource_conflict", Message: "Idempotency key input differs", Status: 409}
	} else if !errors.As(err, &problem) {
		problem = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: problem.Code, Message: problem.Message, Status: problem.Status})
}
