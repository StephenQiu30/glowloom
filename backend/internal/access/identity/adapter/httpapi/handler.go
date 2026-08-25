package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/access/identity/application"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

const refreshCookieName = "lanverse_refresh_token"

type Service interface {
	RequestVerification(context.Context, string) (application.VerificationAccepted, error)
	ConfirmVerification(context.Context, string, string) (application.VerificationConfirmed, error)
	Register(context.Context, application.RegisterCommand) (application.AuthResult, error)
	Login(context.Context, application.LoginCommand) (application.AuthResult, error)
	Refresh(context.Context, string) (application.AuthResult, error)
	Me(context.Context, application.Actor) (application.MeView, error)
	UpdateProfile(context.Context, application.Actor, application.ProfileCommand) (application.MeView, error)
	Logout(context.Context, application.Actor, string) error
	ChangePassword(context.Context, application.Actor, string, string, string) error
	Deactivate(context.Context, application.Actor, string) error
	ListWorkspaces(context.Context, application.Actor, bool) ([]application.WorkspaceView, error)
	GetWorkspace(context.Context, application.Actor, string) (application.WorkspaceView, error)
	CreateWorkspace(context.Context, application.Actor, string, string) (application.WorkspaceView, error)
	UpdateWorkspace(context.Context, application.Actor, string, application.WorkspaceUpdateCommand) (application.WorkspaceView, error)
	SetWorkspaceArchived(context.Context, application.Actor, string, application.WorkspaceStateCommand, bool) (application.WorkspaceView, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	service       Service
	authenticator Authenticator
	validator     *platformvalidation.Validator
	sessionTTL    time.Duration
	secureCookies bool
	newID         func() string
}

func New(service Service, authenticator Authenticator, sessionTTL time.Duration, secureCookies bool, newID func() string) *Handler {
	return &Handler{service: service, authenticator: authenticator, validator: platformvalidation.New(), sessionTTL: sessionTTL, secureCookies: secureCookies, newID: newID}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/registration-verifications", handler.requestVerification)
	mux.HandleFunc("POST /api/v1/auth/registration-verifications/confirm", handler.confirmVerification)
	mux.HandleFunc("POST /api/v1/auth/register", handler.register)
	mux.HandleFunc("POST /api/v1/auth/login", handler.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", handler.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", handler.logout)
	mux.HandleFunc("POST /api/v1/auth/change-password", handler.changePassword)
	mux.HandleFunc("GET /api/v1/me", handler.me)
	mux.HandleFunc("PATCH /api/v1/me", handler.updateMe)
	mux.HandleFunc("POST /api/v1/me/deactivate", handler.deactivate)
	mux.HandleFunc("GET /api/v1/workspaces", handler.listWorkspaces)
	mux.HandleFunc("POST /api/v1/workspaces", handler.createWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces/{workspace_id}", handler.getWorkspace)
	mux.HandleFunc("PATCH /api/v1/workspaces/{workspace_id}", handler.updateWorkspace)
	mux.HandleFunc("POST /api/v1/workspaces/{workspace_id}/archive", handler.archiveWorkspace)
	mux.HandleFunc("POST /api/v1/workspaces/{workspace_id}/restore", handler.restoreWorkspace)
}

type verificationRequest struct {
	Email string `json:"email" validate:"required,email,max=320"`
}
type verificationConfirmRequest struct {
	Email string `json:"email" validate:"required,email,max=320"`
	Code  string `json:"code" validate:"required,len=6,numeric"`
}
type registerRequest struct {
	RegistrationTicket string `json:"registration_ticket" validate:"required,min=43,max=512"`
	Password           string `json:"password" validate:"required,min=12,max=128"`
	DisplayName        string `json:"display_name" validate:"required,max=80"`
}
type loginRequest struct {
	Email    string `json:"email" validate:"required,email,max=320"`
	Password string `json:"password" validate:"required,max=128"`
}
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,max=128"`
	NewPassword     string `json:"new_password" validate:"required,min=12,max=128"`
}
type deactivateRequest struct {
	Confirmation string `json:"confirmation" validate:"required,eq=DEACTIVATE"`
}
type workspaceCreateRequest struct {
	Name string `json:"name" validate:"required,max=120"`
}
type workspaceUpdateRequest struct {
	Name             string `json:"name" validate:"required,max=120"`
	ExpectedRevision int    `json:"expected_revision" validate:"gte=1"`
}
type workspaceStateRequest struct {
	ExpectedRevision int `json:"expected_revision" validate:"gte=1"`
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

type profileRequest struct {
	DisplayName optionalString `json:"display_name"`
	AvatarURL   optionalString `json:"avatar_url"`
}

func (handler *Handler) requestVerification(writer http.ResponseWriter, request *http.Request) {
	var payload verificationRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.RequestVerification(request.Context(), payload.Email)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, 202, map[string]any{"data": map[string]any{"accepted": true, "email_sent": result.EmailSent, "retry_after_seconds": result.RetryAfterSeconds}})
}

func (handler *Handler) confirmVerification(writer http.ResponseWriter, request *http.Request) {
	var payload verificationConfirmRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.ConfirmVerification(request.Context(), payload.Email, payload.Code)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, 200, map[string]any{"data": map[string]any{"registration_ticket": result.RegistrationTicket, "expires_in": result.ExpiresIn}})
}

func (handler *Handler) register(writer http.ResponseWriter, request *http.Request) {
	var payload registerRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Register(request.Context(), application.RegisterCommand{Ticket: payload.RegistrationTicket, Password: payload.Password, DisplayName: payload.DisplayName, TraceID: handler.traceID(request)})
	handler.respondAuth(writer, request, result, err, 201)
}

func (handler *Handler) login(writer http.ResponseWriter, request *http.Request) {
	var payload loginRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Login(request.Context(), application.LoginCommand{Email: payload.Email, Password: payload.Password, TraceID: handler.traceID(request)})
	handler.respondAuth(writer, request, result, err, 200)
}

func (handler *Handler) refresh(writer http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie(refreshCookieName)
	if err != nil {
		handler.clearRefreshCookie(writer)
		handler.writeError(writer, request, &application.Error{Code: application.CodeUnauthenticated, Message: "Invalid credentials", Status: 401, NextAction: "login"})
		return
	}
	result, err := handler.service.Refresh(request.Context(), cookie.Value)
	if err != nil {
		handler.clearRefreshCookie(writer)
	}
	handler.respondAuth(writer, request, result, err, 200)
}

func (handler *Handler) logout(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	if err := handler.service.Logout(request.Context(), actor, handler.traceID(request)); err != nil {
		handler.writeError(writer, request, err)
		return
	}
	handler.clearRefreshCookie(writer)
	platformhttp.WriteJSON(writer, 200, map[string]any{"data": map[string]bool{"revoked": true}})
}

func (handler *Handler) changePassword(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload changePasswordRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	if err := handler.service.ChangePassword(request.Context(), actor, payload.CurrentPassword, payload.NewPassword, handler.traceID(request)); err != nil {
		handler.writeError(writer, request, err)
		return
	}
	handler.clearRefreshCookie(writer)
	platformhttp.WriteJSON(writer, 200, map[string]any{"data": map[string]bool{"revoked": true}})
}

func (handler *Handler) me(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.Me(request.Context(), actor)
	handler.respondMe(writer, request, result, err)
}

func (handler *Handler) updateMe(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload profileRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.UpdateProfile(request.Context(), actor, application.ProfileCommand{DisplayName: payload.DisplayName.Value, DisplayNameSet: payload.DisplayName.Set, AvatarURL: payload.AvatarURL.Value, AvatarURLSet: payload.AvatarURL.Set, TraceID: handler.traceID(request)})
	handler.respondMe(writer, request, result, err)
}

func (handler *Handler) deactivate(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload deactivateRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	if err := handler.service.Deactivate(request.Context(), actor, handler.traceID(request)); err != nil {
		handler.writeError(writer, request, err)
		return
	}
	handler.clearRefreshCookie(writer)
	platformhttp.WriteJSON(writer, 200, map[string]any{"data": map[string]bool{"revoked": true}})
}

func (handler *Handler) listWorkspaces(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	includeArchived, err := strconv.ParseBool(defaultValue(request.URL.Query().Get("include_archived"), "false"))
	if err != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "validation_failed", Message: "Request validation failed", Status: 422})
		return
	}
	result, err := handler.service.ListWorkspaces(request.Context(), actor, includeArchived)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	items := make([]any, len(result))
	for index, workspace := range result {
		items[index] = presentWorkspace(workspace)
	}
	platformhttp.WriteJSON(writer, 200, map[string]any{"data": items})
}

func (handler *Handler) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload workspaceCreateRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.CreateWorkspace(request.Context(), actor, payload.Name, handler.traceID(request))
	handler.respondWorkspace(writer, request, result, err, 201)
}

func (handler *Handler) getWorkspace(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.GetWorkspace(request.Context(), actor, request.PathValue("workspace_id"))
	handler.respondWorkspace(writer, request, result, err, 200)
}

func (handler *Handler) updateWorkspace(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload workspaceUpdateRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.UpdateWorkspace(request.Context(), actor, request.PathValue("workspace_id"), application.WorkspaceUpdateCommand{Name: payload.Name, ExpectedRevision: payload.ExpectedRevision, TraceID: handler.traceID(request)})
	handler.respondWorkspace(writer, request, result, err, 200)
}

func (handler *Handler) archiveWorkspace(writer http.ResponseWriter, request *http.Request) {
	handler.workspaceState(writer, request, true)
}

func (handler *Handler) restoreWorkspace(writer http.ResponseWriter, request *http.Request) {
	handler.workspaceState(writer, request, false)
}

func (handler *Handler) workspaceState(writer http.ResponseWriter, request *http.Request, archived bool) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload workspaceStateRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.SetWorkspaceArchived(request.Context(), actor, request.PathValue("workspace_id"), application.WorkspaceStateCommand{ExpectedRevision: payload.ExpectedRevision, TraceID: handler.traceID(request)}, archived)
	handler.respondWorkspace(writer, request, result, err, 200)
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &application.Error{Code: application.CodeUnauthenticated, Message: "Invalid credentials", Status: 401, NextAction: "login"})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *Handler) respondAuth(writer http.ResponseWriter, request *http.Request, result application.AuthResult, err error, status int) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	handler.setRefreshCookie(writer, result.RefreshToken)
	data := presentMe(result.Me)
	data["access_token"] = result.AccessToken
	data["token_type"] = "bearer"
	data["expires_in"] = result.ExpiresIn
	platformhttp.WriteJSON(writer, status, map[string]any{"data": data})
}

func (handler *Handler) respondMe(writer http.ResponseWriter, request *http.Request, result application.MeView, err error) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, 200, map[string]any{"data": presentMe(result)})
}

func (handler *Handler) respondWorkspace(writer http.ResponseWriter, request *http.Request, result application.WorkspaceView, err error, status int) {
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, status, map[string]any{"data": presentWorkspace(result)})
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: apiError.Code, Message: apiError.Message, Status: apiError.Status, NextAction: apiError.NextAction, Details: apiError.Details})
}

func (handler *Handler) setRefreshCookie(writer http.ResponseWriter, value string) {
	http.SetCookie(writer, &http.Cookie{Name: refreshCookieName, Value: value, Path: "/api/v1/auth", MaxAge: int(handler.sessionTTL.Seconds()), HttpOnly: true, Secure: handler.secureCookies, SameSite: http.SameSiteLaxMode})
}

func (handler *Handler) clearRefreshCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: refreshCookieName, Value: "", Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true, Secure: handler.secureCookies, SameSite: http.SameSiteLaxMode})
}

func (handler *Handler) traceID(request *http.Request) string {
	if value := strings.TrimSpace(request.Header.Get("X-Request-ID")); value != "" {
		return value
	}
	return handler.newID()
}

func presentMe(value application.MeView) map[string]any {
	return map[string]any{"user": map[string]any{"id": value.User.ID, "email": value.User.Email, "display_name": value.User.DisplayName, "avatar_url": value.User.AvatarURL}, "workspace": presentWorkspace(value.Workspace)}
}

func presentWorkspace(value application.WorkspaceView) map[string]any {
	return map[string]any{"id": value.ID, "name": value.Name, "status": value.Status, "role": value.Role, "revision": value.Revision}
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
