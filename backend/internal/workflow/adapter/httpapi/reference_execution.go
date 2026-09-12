package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type ReferenceExecutionStarter interface {
	Start(context.Context, application.Actor, application.StartReferenceExecutionCommand) (domain.WorkflowRun, error)
}

type ReferenceExecutionStartHandler struct {
	starter       ReferenceExecutionStarter
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func NewReferenceExecutionStartHandler(starter ReferenceExecutionStarter, authenticator Authenticator) *ReferenceExecutionStartHandler {
	return &ReferenceExecutionStartHandler{starter: starter, authenticator: authenticator, validator: platformvalidation.New()}
}

func (handler *ReferenceExecutionStartHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{project_id}/reference-executions/{execution_id}/workflow-runs", handler.start)
}

func (handler *ReferenceExecutionStartHandler) start(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "unauthenticated", Message: "Invalid credentials", Status: 401})
		return
	}
	if request.URL.RawQuery != "" || request.URL.ForceQuery {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "validation_failed", Message: "Reference execution start does not accept query parameters", Status: 422})
		return
	}
	var body struct {
		ExecutionHash  string `json:"execution_hash" validate:"required,len=64"`
		IdempotencyKey string `json:"idempotency_key" validate:"required,max=200"`
	}
	raw, readErr := io.ReadAll(http.MaxBytesReader(writer, request.Body, 4096))
	if readErr != nil || canonical.Decode(raw, &body) != nil || handler.validator.Struct(body) != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{Code: "validation_failed", Message: "Invalid Reference execution start body", Status: 422})
		return
	}
	run, err := handler.starter.Start(request.Context(), application.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, application.StartReferenceExecutionCommand{ProjectID: request.PathValue("project_id"), ExecutionRef: gen.GenerationRevisionRef{ID: request.PathValue("execution_id"), Revision: 1, ContentHash: body.ExecutionHash}, IdempotencyKey: body.IdempotencyKey})
	if err != nil {
		problem := platformhttp.Problem{Code: "internal_error", Message: "Internal server error", Status: 500}
		var flowError *application.Error
		var generationError *genapp.Error
		var authoringError *authoringapp.Error
		switch {
		case errors.As(err, &flowError):
			problem.Code, problem.Message, problem.Status = flowError.Code, flowError.Message, flowError.Status
		case errors.As(err, &generationError):
			problem.Code, problem.Message, problem.Status = generationError.Code, generationError.Message, generationError.Status
		case errors.As(err, &authoringError):
			problem.Code, problem.Message, problem.Status = authoringError.Code, authoringError.Message, authoringError.Status
		}
		platformhttp.WriteProblem(writer, request, problem)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusAccepted, map[string]any{"data": map[string]string{"workflow_run_id": run.ID, "status": run.Status}})
}
