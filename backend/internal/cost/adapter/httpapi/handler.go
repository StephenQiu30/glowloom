package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	"github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

type Service interface {
	SetBudget(context.Context, application.Actor, application.SetBudgetCommand) (application.BudgetResult, error)
	GetBudget(context.Context, application.Actor, string) (domain.BudgetPolicy, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	service       Service
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

type setBudgetRequest struct {
	LimitAmount      string `json:"limit_amount" validate:"required"`
	Currency         string `json:"currency" validate:"required,len=3,uppercase"`
	ExpectedRevision int64  `json:"expected_revision" validate:"gte=0"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

type budgetResponse struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	ProjectID   string    `json:"project_id"`
	LimitAmount string    `json:"limit_amount"`
	Currency    string    `json:"currency"`
	Revision    int64     `json:"revision"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func New(service Service, authenticator Authenticator) *Handler {
	return &Handler{service: service, authenticator: authenticator, validator: platformvalidation.New()}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/cost-budget", handler.get)
	mux.HandleFunc("POST /api/v1/projects/{project_id}/cost-budget", handler.set)
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	policy, err := handler.service.GetBudget(request.Context(), actor, request.PathValue("project_id"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": present(policy)})
}

func (handler *Handler) set(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload setBudgetRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	result, err := handler.service.SetBudget(request.Context(), actor, application.SetBudgetCommand{
		ProjectID: request.PathValue("project_id"), LimitAmount: payload.LimitAmount, Currency: payload.Currency,
		ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": present(result.Policy)})
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *Handler) decode(writer http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		writeValidation(writer, request)
		return false
	}
	var extra any
	if decoder.Decode(&extra) == nil {
		writeValidation(writer, request)
		return false
	}
	if err := handler.validator.Struct(target); err != nil {
		writeValidation(writer, request)
		return false
	}
	return true
}

func present(policy domain.BudgetPolicy) budgetResponse {
	return budgetResponse{
		ID: policy.ID, WorkspaceID: policy.WorkspaceID, ProjectID: policy.ProjectID,
		LimitAmount: policy.LimitAmount.StringFixed(6), Currency: policy.Currency, Revision: policy.Revision,
		CreatedBy: policy.CreatedBy, UpdatedBy: policy.UpdatedBy, CreatedAt: policy.CreatedAt, UpdatedAt: policy.UpdatedAt,
	}
}

func writeValidation(writer http.ResponseWriter, request *http.Request) {
	writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Request validation failed", Status: 422})
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
	var requestValue any
	if requestID != "" {
		requestValue = requestID
	}
	var nextAction any
	if apiError.NextAction != "" {
		nextAction = apiError.NextAction
	}
	writeJSON(writer, apiError.Status, map[string]any{"error": map[string]any{
		"code": apiError.Code, "message": apiError.Message, "request_id": requestValue,
		"next_action": nextAction, "details": map[string]any{},
	}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
