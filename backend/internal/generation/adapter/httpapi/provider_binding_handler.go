package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

type ProviderBindingService interface {
	PublishConfiguredImageProviderBinding(
		context.Context,
		application.Actor,
		application.PublishConfiguredImageProviderBindingCommand,
	) (application.ProviderBindingResult, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type ProviderBindingHandler struct {
	service       ProviderBindingService
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

type publishProviderBindingRequest struct {
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
}

type providerBindingResponse struct {
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspace_id"`
	ProjectID     string    `json:"project_id"`
	Capability    string    `json:"capability"`
	ProviderKey   string    `json:"provider_key"`
	ModelKey      string    `json:"model_key"`
	CredentialRef string    `json:"credential_ref"`
	Revision      int64     `json:"revision"`
	ContentHash   string    `json:"content_hash"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	ReceiptID     string    `json:"receipt_id"`
}

func NewProviderBindingHandler(
	service ProviderBindingService,
	authenticator Authenticator,
) *ProviderBindingHandler {
	return &ProviderBindingHandler{
		service: service, authenticator: authenticator, validator: platformvalidation.New(),
	}
}

func (handler *ProviderBindingHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc(
		"POST /api/v1/projects/{project_id}/generation/image-provider-bindings",
		handler.publish,
	)
}

func (handler *ProviderBindingHandler) publish(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload publishProviderBindingRequest
	if !handler.decode(writer, request, &payload) {
		return
	}
	result, err := handler.service.PublishConfiguredImageProviderBinding(
		request.Context(),
		actor,
		application.PublishConfiguredImageProviderBindingCommand{
			ProjectID: request.PathValue("project_id"), IdempotencyKey: payload.IdempotencyKey,
		},
	)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": presentProviderBinding(result)})
}

func (handler *ProviderBindingHandler) actor(
	writer http.ResponseWriter,
	request *http.Request,
) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized,
			NextAction: "login",
		})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *ProviderBindingHandler) decode(
	writer http.ResponseWriter,
	request *http.Request,
	target any,
) bool {
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

func presentProviderBinding(result application.ProviderBindingResult) providerBindingResponse {
	binding := result.Binding
	return providerBindingResponse{
		ID: binding.ID, WorkspaceID: binding.WorkspaceID, ProjectID: binding.ProjectID,
		Capability: binding.Capability, ProviderKey: binding.ProviderKey, ModelKey: binding.ModelKey,
		CredentialRef: binding.CredentialRef, Revision: binding.Revision, ContentHash: binding.ContentHash,
		CreatedBy: binding.CreatedBy, CreatedAt: binding.CreatedAt, ReceiptID: result.Receipt.ID,
	}
}

func writeValidation(writer http.ResponseWriter, request *http.Request) {
	writeError(writer, request, &application.Error{
		Code: "validation_failed", Message: "Request validation failed", Status: http.StatusUnprocessableEntity,
	})
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
	details := apiError.Details
	if details == nil {
		details = map[string]any{}
	}
	writeJSON(writer, apiError.Status, map[string]any{"error": map[string]any{
		"code": apiError.Code, "message": apiError.Message, "request_id": requestValue,
		"next_action": nextAction, "details": details,
	}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
