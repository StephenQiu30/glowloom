package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type Service interface {
	Create(context.Context, application.Actor, application.CreateCommand) (domain.Project, error)
	List(context.Context, application.Actor, application.ListQuery) ([]domain.Project, int, error)
	Get(context.Context, application.Actor, string) (domain.Project, error)
	Update(context.Context, application.Actor, string, application.UpdateCommand) (domain.Project, error)
	UpdateBudget(context.Context, application.Actor, string, application.BudgetCommand) (domain.Project, error)
	SetArchived(context.Context, application.Actor, string, application.StateCommand, bool) (domain.Project, error)
	DeletePreflight(context.Context, application.Actor, string) (application.DeletePreflight, error)
	Delete(context.Context, application.Actor, string, int, string) error
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
	mux.HandleFunc("GET /api/v1/projects", handler.list)
	mux.HandleFunc("POST /api/v1/projects", handler.create)
	mux.HandleFunc("GET /api/v1/projects/{project_id}", handler.get)
	mux.HandleFunc("PATCH /api/v1/projects/{project_id}", handler.update)
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}", handler.delete)
	mux.HandleFunc("POST /api/v1/projects/{project_id}/budget-limit", handler.budget)
	mux.HandleFunc("POST /api/v1/projects/{project_id}/archive", handler.archive)
	mux.HandleFunc("POST /api/v1/projects/{project_id}/restore", handler.restore)
	mux.HandleFunc("POST /api/v1/projects/{project_id}/delete-preflight", handler.preflight)
}

type createRequest struct {
	WorkspaceID      string  `json:"workspace_id" validate:"required"`
	Name             string  `json:"name" validate:"required,max=120"`
	Description      *string `json:"description" validate:"omitempty,max=2000"`
	AspectRatio      string  `json:"aspect_ratio" validate:"omitempty,oneof=9:16 16:9 1:1"`
	Language         string  `json:"language" validate:"omitempty,min=2,max=35"`
	VisualStyle      *string `json:"visual_style" validate:"omitempty,max=200"`
	TargetDurationMS int     `json:"target_duration_ms" validate:"omitempty,gte=1000,lte=7200000"`
	IdempotencyKey   string  `json:"idempotency_key" validate:"required,max=200"`
}
type optionalString struct {
	Set   bool
	Value *string
}

func (value *optionalString) UnmarshalJSON(data []byte) error {
	value.Set = true
	if string(data) == "null" {
		value.Value = nil
		return nil
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded
	return nil
}

type updateRequest struct {
	Name             *string        `json:"name" validate:"omitempty,min=1,max=120"`
	Description      optionalString `json:"description"`
	AspectRatio      *string        `json:"aspect_ratio" validate:"omitempty,oneof=9:16 16:9 1:1"`
	Language         *string        `json:"language" validate:"omitempty,min=2,max=35"`
	VisualStyle      optionalString `json:"visual_style"`
	TargetDurationMS *int           `json:"target_duration_ms" validate:"omitempty,gte=1000,lte=7200000"`
	ExpectedRevision int            `json:"expected_revision" validate:"gte=1"`
	IdempotencyKey   string         `json:"idempotency_key" validate:"required,max=200"`
}
type budgetRequest struct {
	Amount           json.Number `json:"amount" validate:"required"`
	Currency         string      `json:"currency" validate:"len=3,uppercase"`
	ExpectedRevision int         `json:"expected_revision" validate:"gte=1"`
	IdempotencyKey   string      `json:"idempotency_key" validate:"required,max=200"`
}
type stateRequest struct {
	ExpectedRevision int    `json:"expected_revision" validate:"gte=1"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}
type projectResponse struct {
	ID               string        `json:"id"`
	WorkspaceID      string        `json:"workspace_id"`
	Name             string        `json:"name"`
	Description      *string       `json:"description"`
	AspectRatio      string        `json:"aspect_ratio"`
	Language         string        `json:"language"`
	VisualStyle      *string       `json:"visual_style"`
	TargetDurationMS int           `json:"target_duration_ms"`
	BudgetLimit      string        `json:"budget_limit"`
	Currency         string        `json:"currency"`
	Status           domain.Status `json:"status"`
	Revision         int           `json:"revision"`
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{Code: application.CodeUnauthenticated, Message: "Invalid credentials", Status: 401, NextAction: "login"})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}
func (handler *Handler) create(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload createRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	project, err := handler.service.Create(request.Context(), actor, application.CreateCommand{WorkspaceID: payload.WorkspaceID, Name: payload.Name, Description: payload.Description, AspectRatio: payload.AspectRatio, Language: payload.Language, VisualStyle: payload.VisualStyle, TargetDurationMS: payload.TargetDurationMS, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, 201, map[string]any{"data": present(project)})
}
func (handler *Handler) list(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	query := request.URL.Query()
	limit, err := integer(query.Get("limit"), 20)
	if err != nil {
		writeValidation(writer, request)
		return
	}
	offset, err := integer(query.Get("offset"), 0)
	if err != nil {
		writeValidation(writer, request)
		return
	}
	includeArchived, err := boolean(query.Get("include_archived"), false)
	if err != nil {
		writeValidation(writer, request)
		return
	}
	items, total, err := handler.service.List(request.Context(), actor, application.ListQuery{WorkspaceID: query.Get("workspace_id"), IncludeArchived: includeArchived, Search: query.Get("search"), Sort: query.Get("sort"), Order: query.Get("order"), Limit: limit, Offset: offset})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	responses := make([]projectResponse, len(items))
	for index, item := range items {
		responses[index] = present(item)
	}
	writeJSON(writer, 200, map[string]any{"data": map[string]any{"items": responses, "total": total, "limit": limit, "offset": offset}})
}
func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	project, err := handler.service.Get(request.Context(), actor, request.PathValue("project_id"))
	respondProject(writer, request, project, err)
}
func (handler *Handler) update(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload updateRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	project, err := handler.service.Update(request.Context(), actor, request.PathValue("project_id"), application.UpdateCommand{Name: payload.Name, Description: payload.Description.Value, DescriptionSet: payload.Description.Set, AspectRatio: payload.AspectRatio, Language: payload.Language, VisualStyle: payload.VisualStyle.Value, VisualStyleSet: payload.VisualStyle.Set, TargetDurationMS: payload.TargetDurationMS, ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	respondProject(writer, request, project, err)
}
func (handler *Handler) budget(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload budgetRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	project, err := handler.service.UpdateBudget(request.Context(), actor, request.PathValue("project_id"), application.BudgetCommand{Amount: string(payload.Amount), Currency: payload.Currency, ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey})
	respondProject(writer, request, project, err)
}
func (handler *Handler) archive(writer http.ResponseWriter, request *http.Request) {
	handler.state(writer, request, true)
}
func (handler *Handler) restore(writer http.ResponseWriter, request *http.Request) {
	handler.state(writer, request, false)
}
func (handler *Handler) state(writer http.ResponseWriter, request *http.Request, archived bool) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload stateRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	project, err := handler.service.SetArchived(request.Context(), actor, request.PathValue("project_id"), application.StateCommand{ExpectedRevision: payload.ExpectedRevision, IdempotencyKey: payload.IdempotencyKey}, archived)
	respondProject(writer, request, project, err)
}
func (handler *Handler) preflight(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.DeletePreflight(request.Context(), actor, request.PathValue("project_id"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": result})
}
func (handler *Handler) delete(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	revision, err := integer(request.URL.Query().Get("expected_revision"), 0)
	if err != nil || revision < 1 {
		writeValidation(writer, request)
		return
	}
	idempotencyKey := request.URL.Query().Get("idempotency_key")
	if idempotencyKey == "" {
		writeValidation(writer, request)
		return
	}
	if err = handler.service.Delete(request.Context(), actor, request.PathValue("project_id"), revision, idempotencyKey); err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": map[string]bool{"deleted": true}})
}
func respondProject(writer http.ResponseWriter, request *http.Request, project domain.Project, err error) {
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": present(project)})
}
func present(project domain.Project) projectResponse {
	return projectResponse{ID: project.ID, WorkspaceID: project.WorkspaceID, Name: project.Name, Description: project.Description, AspectRatio: project.AspectRatio, Language: project.Language, VisualStyle: project.VisualStyle, TargetDurationMS: project.TargetDurationMS, BudgetLimit: project.BudgetLimit, Currency: project.Currency, Status: project.Status, Revision: project.Revision}
}
func decode(writer http.ResponseWriter, request *http.Request, target any) bool {
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
	return true
}
func (handler *Handler) decode(writer http.ResponseWriter, request *http.Request, target any) bool {
	if !decode(writer, request, target) {
		return false
	}
	if err := handler.validator.Struct(target); err != nil {
		writeValidation(writer, request)
		return false
	}
	return true
}
func integer(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}
func boolean(raw string, fallback bool) (bool, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.ParseBool(raw)
}
func writeValidation(writer http.ResponseWriter, request *http.Request) {
	writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Request validation failed", Status: 422})
}
func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	if apiError.Details == nil {
		apiError.Details = map[string]any{}
	}
	requestID := request.Header.Get("X-Request-ID")
	writeJSON(writer, apiError.Status, map[string]any{"error": map[string]any{"code": apiError.Code, "message": apiError.Message, "request_id": nullable(requestID), "next_action": nullable(apiError.NextAction), "details": apiError.Details}})
}
func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
