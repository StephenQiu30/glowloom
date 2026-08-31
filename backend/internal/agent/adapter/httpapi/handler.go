package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type CandidateSource interface {
	GetCandidate(context.Context, string, string) (agentapp.Candidate, error)
}

type ProjectSource interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	candidates    CandidateSource
	projects      ProjectSource
	authenticator Authenticator
}

func New(candidates CandidateSource, projects ProjectSource, authenticator Authenticator) *Handler {
	return &Handler{candidates: candidates, projects: projects, authenticator: authenticator}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(
		"GET /api/projects/{project_id}/scene-analysis-candidates/{candidate_id}",
		handler.getCandidate,
	)
}

func (handler *Handler) getCandidate(writer http.ResponseWriter, request *http.Request) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return
	}
	projectID := request.PathValue("project_id")
	if _, err = handler.projects.Get(request.Context(), projectapp.Actor{
		UserID: claims.UserID, TokenVersion: claims.TokenVersion,
	}, projectID); err != nil {
		handler.writeError(writer, request, err)
		return
	}
	candidate, err := handler.candidates.GetCandidate(
		request.Context(), projectID, request.PathValue("candidate_id"),
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": candidate})
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	problem := platformhttp.Problem{Code: "internal_error", Message: "Internal server error", Status: 500}
	var agentError *agentapp.Error
	var projectError *projectapp.Error
	switch {
	case errors.Is(err, agentapp.ErrNotFound):
		problem = platformhttp.Problem{Code: "not_found", Message: "Scene Analysis candidate not found", Status: 404}
	case errors.As(err, &agentError):
		problem = platformhttp.Problem{Code: agentError.Code, Message: agentError.Message, Status: 409}
	case errors.As(err, &projectError):
		problem = platformhttp.Problem{
			Code: projectError.Code, Message: projectError.Message, Status: projectError.Status,
			NextAction: projectError.NextAction,
		}
	}
	platformhttp.WriteProblem(writer, request, problem)
}
