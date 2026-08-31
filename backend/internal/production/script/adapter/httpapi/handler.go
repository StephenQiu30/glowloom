package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

type Service interface {
	Preview(context.Context, application.Actor, string, string) (application.Preview, error)
	Import(context.Context, application.Actor, application.ImportCommand) (domain.Analysis, error)
	GetRevision(context.Context, application.Actor, string) (domain.Analysis, error)
	GetCurrentAnalysis(context.Context, application.Actor, string) (domain.Analysis, error)
	ListDocuments(context.Context, application.Actor, string, int, int) ([]domain.Document, int, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type SourceService interface {
	Accept(context.Context, application.Actor, application.AcceptSourceCommand) (domain.AcceptedSource, error)
	GetExact(context.Context, application.Actor, string, string) (domain.AcceptedSource, error)
}

type Handler struct {
	service       Service
	sources       SourceService
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func New(service Service, sources SourceService, authenticator Authenticator) *Handler {
	return &Handler{service: service, sources: sources, authenticator: authenticator, validator: platformvalidation.New()}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{project_id}/script-import-previews", handler.preview)
	mux.HandleFunc("POST /api/projects/{project_id}/script-imports", handler.importDocument)
	mux.HandleFunc("GET /api/projects/{project_id}/current-script-document", handler.getCurrentAnalysis)
	mux.HandleFunc("GET /api/projects/{project_id}/script-documents", handler.listDocuments)
	mux.HandleFunc("GET /api/document-revisions/{revision_id}", handler.getRevision)
	mux.HandleFunc("POST /api/projects/{project_id}/script-sources", handler.acceptSource)
	mux.HandleFunc("GET /api/projects/{project_id}/script-sources/{revision_id}", handler.getSource)
}

type previewRequest struct {
	MediaVersionID string `json:"media_version_id" validate:"required,uuid"`
}

type importRequest struct {
	InputType         string  `json:"input_type" validate:"required,oneof=text media"`
	Title             string  `json:"title" validate:"required,max=120"`
	Text              *string `json:"text"`
	MediaVersionID    *string `json:"media_version_id" validate:"omitempty,uuid"`
	Language          string  `json:"language" validate:"required,max=35"`
	RightsDeclaration string  `json:"rights_declaration" validate:"required,max=1000"`
	IdempotencyKey    string  `json:"idempotency_key" validate:"required,max=200"`
}

type acceptSourceRequest struct {
	DocumentRevisionID   string  `json:"document_revision_id" validate:"required,uuid"`
	ExpectedHeadRevision *int64  `json:"expected_head_revision" validate:"required,gte=0"`
	ExpectedHeadHash     *string `json:"expected_head_hash" validate:"omitempty,len=64,lowercase,hexadecimal"`
	IdempotencyKey       string  `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) acceptSource(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload acceptSourceRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.sources.Accept(request.Context(), actor, application.AcceptSourceCommand{
		ProjectID: request.PathValue("project_id"), DocumentRevisionID: payload.DocumentRevisionID,
		ExpectedHeadRevision: *payload.ExpectedHeadRevision, ExpectedHeadHash: payload.ExpectedHeadHash,
		IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": result})
}

func (handler *Handler) getSource(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.sources.GetExact(
		request.Context(), actor, request.PathValue("project_id"), request.PathValue("revision_id"),
	)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": result})
}

func (handler *Handler) preview(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload previewRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Preview(request.Context(), actor, request.PathValue("project_id"), payload.MediaVersionID)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"media_version_id": result.MediaVersionID, "raw_text": result.RawText, "raw_hash": result.RawHash, "codepoint_count": result.CodepointCount}})
}

func (handler *Handler) importDocument(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload importRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Import(request.Context(), actor, application.ImportCommand{ProjectID: request.PathValue("project_id"), InputType: payload.InputType, Title: payload.Title, Text: payload.Text, MediaVersionID: payload.MediaVersionID, Language: payload.Language, RightsDeclaration: payload.RightsDeclaration, IdempotencyKey: payload.IdempotencyKey})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": presentAnalysis(result)})
}

func (handler *Handler) getRevision(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.GetRevision(request.Context(), actor, request.PathValue("revision_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentAnalysis(result)})
}

func (handler *Handler) getCurrentAnalysis(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	result, err := handler.service.GetCurrentAnalysis(request.Context(), actor, request.PathValue("project_id"))
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentAnalysis(result)})
}

func (handler *Handler) listDocuments(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	limit, err := integer(request.URL.Query().Get("limit"), 20)
	if err != nil {
		handler.writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Invalid pagination", Status: 422})
		return
	}
	offset, err := integer(request.URL.Query().Get("offset"), 0)
	if err != nil {
		handler.writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Invalid pagination", Status: 422})
		return
	}
	items, total, err := handler.service.ListDocuments(request.Context(), actor, request.PathValue("project_id"), limit, offset)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	responseItems := make([]map[string]any, len(items))
	for index, item := range items {
		responseItems[index] = presentDocument(item)
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"items": responseItems, "total": total, "limit": limit, "offset": offset}})
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

func presentAnalysis(value domain.Analysis) map[string]any {
	blocks := make([]map[string]any, len(value.Revision.Blocks))
	for index, block := range value.Revision.Blocks {
		blocks[index] = map[string]any{"id": block.ID, "document_revision_id": block.DocumentRevisionID, "position": block.Position, "kind": block.Kind, "source_start": block.SourceStart, "source_end": block.SourceEnd, "text_hash": block.TextHash, "metadata": block.Metadata}
	}
	issues := make([]map[string]any, len(value.Revision.Issues))
	for index, issue := range value.Revision.Issues {
		issues[index] = map[string]any{"id": issue.ID, "document_revision_id": issue.DocumentRevisionID, "position": issue.Position, "code": issue.Code, "severity": issue.Severity, "source_start": issue.SourceStart, "source_end": issue.SourceEnd, "line_number": issue.LineNumber, "column_number": issue.ColumnNumber, "next_action": issue.NextAction, "details": issue.Details}
	}
	return map[string]any{"document": presentDocument(value.Document), "revision": presentRevision(value.Revision), "blocks": blocks, "issues": issues}
}

func presentDocument(value domain.Document) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID, "title": value.Title, "source_type": value.SourceType, "source_media_version_id": value.SourceMediaVersionID, "language": value.Language, "rights_declaration": value.RightsDeclaration, "status": value.Status, "revision": value.Revision, "created_by": value.CreatedBy, "created_at": value.CreatedAt}
}

func presentRevision(value domain.Revision) map[string]any {
	return map[string]any{"id": value.ID, "workspace_id": value.WorkspaceID, "document_id": value.DocumentID, "version_no": value.VersionNo, "source_type": value.SourceType, "source_media_version_id": value.SourceMediaVersionID, "raw_text": value.RawText, "raw_hash": value.RawHash, "normalized_text": value.NormalizedText, "normalized_hash": value.NormalizedHash, "normalizer_version": value.NormalizerVersion, "normalization_map": value.NormalizationMap, "codepoint_count": value.CodepointCount, "analysis_status": value.AnalysisStatus, "analyzer_version": value.AnalyzerVersion, "created_by": value.CreatedBy, "created_at": value.CreatedAt}
}

func integer(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}
