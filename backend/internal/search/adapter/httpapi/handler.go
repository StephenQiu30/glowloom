package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

type Service interface {
	SearchScripts(context.Context, searchapp.Actor, searchapp.Query) (search.Result, error)
	SearchStoryGraph(context.Context, searchapp.Actor, searchapp.Query) (search.Result, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	service Service
	auth    Authenticator
}

func New(service Service, auth Authenticator) *Handler { return &Handler{service: service, auth: auth} }

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/search/scripts", handler.scripts)
	mux.HandleFunc("GET /api/projects/{project_id}/search/storygraph", handler.storyGraph)
}

func (handler *Handler) scripts(writer http.ResponseWriter, request *http.Request) {
	handler.search(writer, request, handler.service.SearchScripts)
}

func (handler *Handler) storyGraph(writer http.ResponseWriter, request *http.Request) {
	handler.search(writer, request, handler.service.SearchStoryGraph)
}

func (handler *Handler) search(writer http.ResponseWriter, request *http.Request, operation func(context.Context, searchapp.Actor, searchapp.Query) (search.Result, error)) {
	claims, err := handler.auth.Authenticate(request)
	if err != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login"})
		return
	}
	limit, err := strconv.Atoi(request.URL.Query().Get("limit"))
	if err != nil || strings.TrimSpace(request.URL.Query().Get("limit")) == "" {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "validation_failed", Message: "Request validation failed", Status: http.StatusUnprocessableEntity})
		return
	}
	result, err := operation(request.Context(), searchapp.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, searchapp.Query{
		ProjectID: request.PathValue("project_id"), Text: request.URL.Query().Get("q"), Limit: limit,
	})
	if err != nil {
		var value *searchapp.Error
		if !errors.As(err, &value) {
			platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "internal_error", Message: "Internal server error", Status: http.StatusInternalServerError})
			return
		}
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: value.Code, Message: value.Message, Status: value.Status, NextAction: value.NextAction, Details: value.Details})
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": result})
}
