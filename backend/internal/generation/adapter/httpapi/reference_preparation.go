package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

type ReferenceGenerationAuthorizer interface {
	AuthorizeInitial(context.Context, application.Actor, application.AuthorizeInitialReferenceGenerationCommand) (domain.ReferenceGenerationAuthorization, error)
}

type ReferenceGenerationTargetBuilder interface {
	BuildInitial(context.Context, application.Actor, application.BuildReferenceGenerationTargetCommand) (application.ReferenceGenerationTarget, error)
}

type ReferenceExecutionAuthorizer interface {
	AuthorizeInitial(context.Context, application.Actor, application.AuthorizeInitialReferenceExecutionCommand) (domain.ReferenceExecutionAuthorization, error)
}

type ReferenceExecutionPreparer interface {
	PrepareInitial(context.Context, application.Actor, application.PrepareInitialReferenceExecutionCommand) (domain.ReferenceExecution, error)
}

type ReferencePreparationHandler struct {
	generation    ReferenceGenerationAuthorizer
	targets       ReferenceGenerationTargetBuilder
	execution     ReferenceExecutionAuthorizer
	preparation   ReferenceExecutionPreparer
	authenticator Authenticator
	validator     *validation.Validator
}

func NewReferencePreparationHandler(generation ReferenceGenerationAuthorizer, targets ReferenceGenerationTargetBuilder, execution ReferenceExecutionAuthorizer, preparation ReferenceExecutionPreparer, authenticator Authenticator) (*ReferencePreparationHandler, error) {
	if generation == nil || targets == nil || execution == nil || preparation == nil || authenticator == nil {
		return nil, errors.New("Reference preparation HTTP dependencies are required")
	}
	return &ReferencePreparationHandler{generation, targets, execution, preparation, authenticator, validation.New()}, nil
}

func (handler *ReferencePreparationHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{project_id}/reference-targets/{target_version_id}/generation-authorizations", handler.authorizeGeneration)
	mux.HandleFunc("POST /api/projects/{project_id}/reference-generation-targets", handler.buildTarget)
	mux.HandleFunc("POST /api/projects/{project_id}/reference-generation-targets/{generation_target_id}/execution-authorizations", handler.authorizeExecution)
	mux.HandleFunc("POST /api/projects/{project_id}/reference-generation-targets/{generation_target_id}/executions", handler.prepareExecution)
}

func (handler *ReferencePreparationHandler) readCommand(writer http.ResponseWriter, request *http.Request, body any) (application.Actor, bool) {
	writer.Header().Set("Cache-Control", "no-store")
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		writeError(writer, request, &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401})
		return application.Actor{}, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 16*1024))
	if err != nil || request.URL.RawQuery != "" || request.URL.ForceQuery || canonical.Decode(raw, body) != nil || handler.validator.Struct(body) != nil {
		writeError(writer, request, &application.Error{Code: "validation_failed", Message: "Invalid Reference preparation request", Status: 422})
		return application.Actor{}, false
	}
	return application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *ReferencePreparationHandler) authorizeGeneration(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		WorkspaceID          string `json:"workspace_id" validate:"required,uuid"`
		PlanVersionID        string `json:"plan_version_id" validate:"required,uuid"`
		PlanContentHash      string `json:"plan_content_hash" validate:"required,len=64"`
		TargetContentHash    string `json:"target_content_hash" validate:"required,len=64"`
		BriefRevisionID      string `json:"brief_revision_id" validate:"required,uuid"`
		BriefRevisionHash    string `json:"brief_revision_hash" validate:"required,len=64"`
		CandidateBundleCount int    `json:"candidate_bundle_count" validate:"min=1,max=4"`
		IdempotencyKey       string `json:"idempotency_key" validate:"required,max=200"`
	}
	actor, ok := handler.readCommand(writer, request, &body)
	if !ok {
		return
	}
	value, err := handler.generation.AuthorizeInitial(request.Context(), actor, application.AuthorizeInitialReferenceGenerationCommand{
		WorkspaceID: body.WorkspaceID, ProjectID: request.PathValue("project_id"), PlanVersionID: body.PlanVersionID, PlanContentHash: body.PlanContentHash,
		TargetVersionID: request.PathValue("target_version_id"), TargetContentHash: body.TargetContentHash, BriefRevisionID: body.BriefRevisionID,
		BriefRevisionHash: body.BriefRevisionHash, CandidateBundleCount: body.CandidateBundleCount, IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": map[string]any{"generation_authorization_ref": domain.GenerationActionRef{ID: value.HumanActionRef, ContentHash: value.ContentHash}}})
}

// Keep wire naming separate from the application command's persisted hash shape.
type referenceSlotPolicyBody struct {
	ViewRole          string   `json:"view_role" validate:"required,max=100"`
	AllowedMediaTypes []string `json:"allowed_media_types" validate:"required,min=1,max=2,dive,oneof=image/png image/jpeg"`
	AspectRatio       string   `json:"aspect_ratio" validate:"required,max=20"`
	MinWidth          int      `json:"min_width" validate:"min=1,max=8192"`
	MinHeight         int      `json:"min_height" validate:"min=1,max=8192"`
	MaxBytes          int64    `json:"max_bytes" validate:"min=1,max=10485760"`
}

func (handler *ReferencePreparationHandler) buildTarget(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		WorkspaceID                string                     `json:"workspace_id" validate:"required,uuid"`
		GenerationAuthorizationRef domain.GenerationActionRef `json:"generation_authorization_ref" validate:"required"`
		BriefRevisionID            string                     `json:"brief_revision_id" validate:"required,uuid"`
		BriefRevisionHash          string                     `json:"brief_revision_hash" validate:"required,len=64"`
		SlotPolicies               []referenceSlotPolicyBody  `json:"slot_policies" validate:"required,min=1,max=16,dive"`
		IdempotencyKey             string                     `json:"idempotency_key" validate:"required,max=200"`
	}
	actor, ok := handler.readCommand(writer, request, &body)
	if !ok {
		return
	}
	policies := make([]application.ReferenceOutputSlotPolicy, len(body.SlotPolicies))
	for index, policy := range body.SlotPolicies {
		policies[index] = application.ReferenceOutputSlotPolicy{ViewRole: policy.ViewRole, AllowedMediaTypes: policy.AllowedMediaTypes, AspectRatio: policy.AspectRatio, MinWidth: policy.MinWidth, MinHeight: policy.MinHeight, MaxBytes: policy.MaxBytes}
	}
	value, err := handler.targets.BuildInitial(request.Context(), actor, application.BuildReferenceGenerationTargetCommand{
		WorkspaceID: body.WorkspaceID, ProjectID: request.PathValue("project_id"), AuthorizationID: body.GenerationAuthorizationRef.ID,
		AuthorizationHash: body.GenerationAuthorizationRef.ContentHash, BriefRevisionID: body.BriefRevisionID, BriefRevisionHash: body.BriefRevisionHash,
		ExpectedHeadRevision: 0, SlotPolicies: policies, IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": map[string]any{"generation_target_ref": domain.GenerationRevisionRef{ID: value.ID, Revision: value.Revision, ContentHash: value.ContentHash}}})
}

func (handler *ReferencePreparationHandler) authorizeExecution(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		WorkspaceID                string                       `json:"workspace_id" validate:"required,uuid"`
		TargetHash                 string                       `json:"target_hash" validate:"required,len=64"`
		SelectedProviderBindingRef domain.GenerationRevisionRef `json:"selected_provider_binding_ref" validate:"required"`
		IdempotencyKey             string                       `json:"idempotency_key" validate:"required,max=200"`
	}
	actor, ok := handler.readCommand(writer, request, &body)
	if !ok {
		return
	}
	value, err := handler.execution.AuthorizeInitial(request.Context(), actor, application.AuthorizeInitialReferenceExecutionCommand{
		WorkspaceID: body.WorkspaceID, ProjectID: request.PathValue("project_id"),
		TargetRef:                  domain.GenerationRevisionRef{ID: request.PathValue("generation_target_id"), Revision: 1, ContentHash: body.TargetHash},
		SelectedProviderBindingRef: body.SelectedProviderBindingRef, IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": map[string]any{"execution_authorization_ref": domain.GenerationActionRef{ID: value.HumanActionRef, ContentHash: value.ContentHash}}})
}

func (handler *ReferencePreparationHandler) prepareExecution(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		WorkspaceID               string                     `json:"workspace_id" validate:"required,uuid"`
		TargetHash                string                     `json:"target_hash" validate:"required,len=64"`
		ExecutionAuthorizationRef domain.GenerationActionRef `json:"execution_authorization_ref" validate:"required"`
		IdempotencyKey            string                     `json:"idempotency_key" validate:"required,max=200"`
	}
	actor, ok := handler.readCommand(writer, request, &body)
	if !ok {
		return
	}
	value, err := handler.preparation.PrepareInitial(request.Context(), actor, application.PrepareInitialReferenceExecutionCommand{
		WorkspaceID: body.WorkspaceID, ProjectID: request.PathValue("project_id"),
		TargetRef:        domain.GenerationRevisionRef{ID: request.PathValue("generation_target_id"), Revision: 1, ContentHash: body.TargetHash},
		AuthorizationRef: body.ExecutionAuthorizationRef, ExpectedHeadRevision: 0, IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusCreated, map[string]any{"data": map[string]any{"execution_ref": domain.GenerationRevisionRef{ID: value.ID, Revision: value.Revision, ContentHash: value.ContentHash}}})
}
