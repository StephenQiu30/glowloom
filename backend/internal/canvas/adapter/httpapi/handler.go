package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

type Service interface {
	Get(context.Context, application.Actor, string) (domain.Document, error)
	Apply(context.Context, application.Actor, application.ApplyCommand) (application.ApplyResult, error)
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
	mux.HandleFunc("GET /api/projects/{project_id}/canvas", handler.get)
	mux.HandleFunc("POST /api/projects/{project_id}/canvas/operations", handler.apply)
}

type applyRequest struct {
	Operations     []domain.Operation `json:"operations" validate:"required,min=1,max=100"`
	IdempotencyKey string             `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	document, err := handler.service.Get(request.Context(), actor, request.PathValue("project_id"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": document})
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
	result, err := handler.service.Apply(request.Context(), actor, application.ApplyCommand{
		ProjectID: request.PathValue("project_id"), Operations: payload.Operations,
		IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": result})
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{
		Code: apiError.Code, Message: apiError.Message, Status: apiError.Status,
		NextAction: apiError.NextAction,
	})
}
