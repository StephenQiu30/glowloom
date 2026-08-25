package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

type Service interface {
	Initialize(context.Context, application.Actor, application.InitializeCommand) (application.Initialization, error)
	Complete(context.Context, application.Actor, string) (application.Completion, error)
	GetVersion(context.Context, application.Actor, string) (domain.MediaVersion, error)
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
	mux.HandleFunc("POST /api/v1/media/uploads", handler.initialize)
	mux.HandleFunc("POST /api/v1/media/uploads/{upload_session_id}/complete", handler.complete)
	mux.HandleFunc("GET /api/v1/media/{version_id}", handler.getVersion)
}

type initializeRequest struct {
	WorkspaceID    string `json:"workspace_id" validate:"required,uuid"`
	Kind           string `json:"kind" validate:"required,eq=document"`
	Filename       string `json:"filename" validate:"required,max=255"`
	SizeBytes      int64  `json:"size_bytes" validate:"gte=1,lte=20971520"`
	MIMEType       string `json:"mime_type" validate:"required,max=120"`
	SHA256         string `json:"sha256" validate:"required,len=64,hexadecimal"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) initialize(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload initializeRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Initialize(request.Context(), actor, application.InitializeCommand{WorkspaceID: payload.WorkspaceID, Kind: payload.Kind, Filename: payload.Filename, SizeBytes: payload.SizeBytes, MIMEType: payload.MIMEType, SHA256: payload.SHA256, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": map[string]any{"upload_session": presentUpload(result.Session), "upload": map[string]any{"url": result.Upload.URL, "method": result.Upload.Method, "headers": result.Upload.Headers, "expires_at": result.Upload.ExpiresAt}}})
}

func (handler *Handler) complete(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.Complete(request.Context(), actor, request.PathValue("upload_session_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"media_object": presentObject(result.Object), "version": presentVersion(result.Version), "probe_task": presentTask(result.Task)}})
}

func (handler *Handler) getVersion(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.GetVersion(request.Context(), actor, request.PathValue("version_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentVersion(result)})
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (application.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{Code: "internal_error", Message: "Internal server error", Status: 500}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: apiError.Code, Message: apiError.Message, Status: apiError.Status, NextAction: apiError.NextAction, Details: apiError.Details})
}

func presentUpload(value domain.UploadSession) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "media_object_id": value.MediaObjectID, "status": value.Status, "kind": value.Kind, "filename": value.Filename, "size_bytes": value.SizeBytes, "mime_type": value.MIMEType, "sha256": value.SHA256, "expires_at": value.ExpiresAt}
}

func presentObject(value domain.MediaObject) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "kind": value.Kind, "source_type": value.SourceType, "status": value.Status, "current_version_id": value.CurrentVersionID, "revision": value.Revision}
}

func presentVersion(value domain.MediaVersion) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "media_object_id": value.MediaObjectID, "media_object_kind": value.MediaObject.Kind, "media_object_source_type": value.MediaObject.SourceType, "media_object_status": value.MediaObject.Status, "media_object_current_version_id": value.MediaObject.CurrentVersionID, "media_object_revision": value.MediaObject.Revision, "version_no": value.VersionNo, "filename": value.Filename, "sha256": value.SHA256, "size_bytes": value.SizeBytes, "mime_type": value.MIMEType, "probe_status": value.ProbeStatus, "probe_attempt": value.ProbeAttempt, "probe_error_code": value.ProbeErrorCode, "probe_error_summary": value.ProbeSummary, "probe_next_action": value.ProbeNextAction, "width": value.Width, "height": value.Height, "duration_ms": value.DurationMS, "codec": value.Codec, "container": value.Container, "created_at": value.CreatedAt}
}

func presentTask(value domain.Task) map[string]any {
	var scope any = map[string]any{"episode_id": nil, "render_snapshot_id": nil, "usage_type": nil, "usage_id": nil, "input_version_id": nil, "input_hash": nil}
	_ = json.Unmarshal(value.Scope, &scope)
	var taskError any
	if len(value.Error) > 0 {
		_ = json.Unmarshal(value.Error, &taskError)
	}
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "task_type": value.TaskType, "request_type": value.RequestType, "request_id": value.RequestID, "scope": scope, "status": value.Status, "progress_stage": value.ProgressStage, "error": taskError, "next_action": value.NextAction, "cancel_status": value.CancelStatus, "revision": value.Revision}
}
