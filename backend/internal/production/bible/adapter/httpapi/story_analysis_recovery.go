package httpapi

import (
	"context"
	"errors"
	"net/http"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
)

type StoryAnalysisRecoveryService interface {
	Recover(
		context.Context,
		application.Actor,
		application.StoryAnalysisRecoveryCommand,
	) (application.StoryAnalysisRecovery, error)
}

type StoryAnalysisRecoveryHandler struct {
	service       StoryAnalysisRecoveryService
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func NewStoryAnalysisRecovery(
	service StoryAnalysisRecoveryService,
	authenticator Authenticator,
) *StoryAnalysisRecoveryHandler {
	return &StoryAnalysisRecoveryHandler{
		service: service, authenticator: authenticator, validator: platformvalidation.New(),
	}
}

func (handler *StoryAnalysisRecoveryHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc(
		"POST /api/v1/workflow-runs/{run_id}/story-analysis-recoveries",
		handler.recover,
	)
}

type storyAnalysisRecoveryRequest struct {
	NodeRunID      string `json:"node_run_id" validate:"required,uuid"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *StoryAnalysisRecoveryHandler) recover(writer http.ResponseWriter, request *http.Request) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &application.Error{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return
	}
	var payload storyAnalysisRecoveryRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	result, err := handler.service.Recover(request.Context(), application.Actor{
		UserID: claims.UserID, TokenVersion: claims.TokenVersion,
	}, application.StoryAnalysisRecoveryCommand{
		WorkflowRunID: request.PathValue("run_id"), NodeRunID: payload.NodeRunID,
		IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"receipt_id": result.ReceiptID, "workflow_run_id": result.WorkflowRunID,
			"node_run_id": result.NodeRunID, "invocation_id": result.InvocationID,
			"stage": result.Stage, "shard_key": result.ShardKey,
			"status": result.Status, "failure_code": result.FailureCode,
			"previous_claim_version": result.PreviousClaimVersion,
		},
	})
}

func (handler *StoryAnalysisRecoveryHandler) writeError(
	writer http.ResponseWriter,
	request *http.Request,
	err error,
) {
	var apiError *application.Error
	if !errors.As(err, &apiError) {
		apiError = &application.Error{
			Code: "internal_error", Message: "Internal server error", Status: http.StatusInternalServerError,
		}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{
		Code: apiError.Code, Message: apiError.Message, Status: apiError.Status,
		NextAction: apiError.NextAction, Details: apiError.Details,
	})
}
